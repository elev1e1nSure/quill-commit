package watcher

import (
	"fmt"
	"time"

	"quill-commit/ai"
	"quill-commit/config"
	"quill-commit/git"
)

type EventKind int

const (
	EventCheck EventKind = iota
	EventDecision
	EventCommit
	EventForced
	EventError
	EventSkip
)

type Event struct {
	Kind    EventKind
	Message string
	Time    time.Time
}

func newEvent(kind EventKind, msg string) Event {
	return Event{Kind: kind, Message: msg, Time: time.Now()}
}

type Watcher struct {
	cfg    config.Config
	apiKey string
	Events chan Event

	prevDiff     string
	delayCounter int
}

func New(cfg config.Config, apiKey string) *Watcher {
	return &Watcher{
		cfg:    cfg,
		apiKey: apiKey,
		Events: make(chan Event, 64),
	}
}

func (w *Watcher) Run() {
	ticker := time.NewTicker(time.Duration(w.cfg.Interval) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		w.tick()
	}
}

func (w *Watcher) tick() {
	diff, err := git.Diff()
	if err != nil {
		w.emit(EventError, fmt.Sprintf("git diff: %s", err))
		return
	}

	w.emit(EventCheck, "checking diff")

	if diff == "" {
		w.emit(EventSkip, "diff empty, waiting")
		w.prevDiff = ""
		return
	}

	if diff != w.prevDiff {
		w.emit(EventSkip, "diff changed, waiting for stabilization")
		w.prevDiff = diff
		return
	}

	// diff is stable — enter delay/retry loop
	w.delayLoop(diff)
}

// delayLoop asks the model and handles commit: false delays without recursion.
// After each sleep it re-checks the diff; if it changed, stabilization resets.
func (w *Watcher) delayLoop(stableDiff string) {
	for {
		decision, err := ai.Ask(stableDiff, w.cfg.Model, w.apiKey)
		if err != nil {
			// network error: skip, do not count toward delay counter
			w.emit(EventError, fmt.Sprintf("ai error (skipping): %s", err))
			return
		}

		if decision.Commit {
			w.emit(EventDecision, fmt.Sprintf("model says commit: %s", decision.Message))
			w.doCommit(decision.Message)
			return
		}

		w.delayCounter++
		w.emit(EventDecision, fmt.Sprintf("model says wait %dm (delay %d/%d)", decision.Delay, w.delayCounter, w.cfg.MaxDelays))

		if w.delayCounter >= w.cfg.MaxDelays {
			w.emit(EventForced, "max delays reached, forcing commit")
			w.doCommit("auto: forced commit")
			return
		}

		time.Sleep(time.Duration(decision.Delay) * time.Minute)

		// re-check diff after sleep: if code changed, stabilization resets
		currentDiff, err := git.Diff()
		if err != nil {
			w.emit(EventError, fmt.Sprintf("git diff after delay: %s", err))
			return
		}
		if currentDiff != stableDiff {
			w.emit(EventSkip, "diff changed during delay, resetting stabilization")
			w.prevDiff = currentDiff
			w.delayCounter = 0
			return
		}
	}
}

func (w *Watcher) doCommit(message string) {
	if err := git.Add(); err != nil {
		w.emit(EventError, fmt.Sprintf("git add: %s", err))
		return
	}
	if err := git.Commit(message); err != nil {
		w.emit(EventError, fmt.Sprintf("git commit: %s", err))
		return
	}
	w.emit(EventCommit, message)
	w.prevDiff = ""
	w.delayCounter = 0
}

func (w *Watcher) emit(kind EventKind, msg string) {
	select {
	case w.Events <- newEvent(kind, msg):
	default:
	}
}
