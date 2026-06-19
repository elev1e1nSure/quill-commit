package watcher

import (
	"fmt"
	"os"
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
	EventDelay
)

var EventKindNames = map[EventKind]string{
	EventCheck:    "EventCheck",
	EventDecision: "EventDecision",
	EventCommit:   "EventCommit",
	EventForced:   "EventForced",
	EventError:    "EventError",
	EventSkip:     "EventSkip",
	EventDelay:    "EventDelay",
}

type Event struct {
	Kind    EventKind
	Message string
	Time    time.Time
}

func newEvent(kind EventKind, msg string) Event {
	return Event{Kind: kind, Message: msg, Time: time.Now()}
}

type gitOps interface {
	Diff() (string, error)
	Add() error
	Commit(message string) error
}

type aiOps interface {
	Ask(diff, model, apiKey string) (ai.Decision, error)
}

type realGit struct{}

func (realGit) Diff() (string, error)        { return git.Diff() }
func (realGit) Add() error                   { return git.Add() }
func (realGit) Commit(message string) error  { return git.Commit(message) }

type realAI struct{}

func (realAI) Ask(diff, model, apiKey string) (ai.Decision, error) {
	return ai.Ask(diff, model, apiKey)
}

type Watcher struct {
	cfg    config.Config
	apiKey string
	Events chan Event

	git gitOps
	ai  aiOps

	prevDiff     string
	delayCounter int
}

func New(cfg config.Config, apiKey string) *Watcher {
	return &Watcher{
		cfg:    cfg,
		apiKey: apiKey,
		Events: make(chan Event, 64),
		git:    realGit{},
		ai:     realAI{},
	}
}

func (w *Watcher) Run() {
	ticker := time.NewTicker(time.Duration(w.cfg.Interval * float64(time.Minute)))
	defer ticker.Stop()

	for range ticker.C {
		w.tick()
	}
}

func (w *Watcher) tick() {
	diff, err := w.git.Diff()
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

	for diff != w.prevDiff {
		w.emit(EventSkip, fmt.Sprintf("diff changed, re-checking in %s", formatDuration(w.cfg.Stabilize)))
		w.prevDiff = diff
		time.Sleep(time.Duration(w.cfg.Stabilize * float64(time.Minute)))
		diff, err = w.git.Diff()
		if err != nil {
			w.emit(EventError, fmt.Sprintf("git diff: %s", err))
			return
		}
		if diff == "" {
			w.emit(EventSkip, "diff cleared during stabilization")
			w.prevDiff = ""
			return
		}
	}

	w.delayLoop(diff)
}

func formatDuration(minutes float64) string {
	d := time.Duration(minutes * float64(time.Minute))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

// delayLoop asks the model and handles commit: false delays without recursion.
// After each sleep it re-checks the diff; if it changed, stabilization resets.
func (w *Watcher) delayLoop(stableDiff string) {
	for {
		decision, err := w.ai.Ask(stableDiff, w.cfg.Model, w.apiKey)
		if err != nil {
			// network error: do not count toward delay counter; reset so next
			// stabilization cycle starts clean
			w.emit(EventError, fmt.Sprintf("ai error (skipping): %s", err))
			w.delayCounter = 0
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

		w.emit(EventDelay, fmt.Sprintf("sleeping %dm before retry", decision.Delay))
		time.Sleep(time.Duration(decision.Delay) * time.Minute)

		currentDiff, err := w.git.Diff()
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
	diff, err := w.git.Diff()
	if err != nil {
		w.emit(EventError, fmt.Sprintf("git diff before commit: %s", err))
		return
	}
	if diff == "" {
		w.emit(EventSkip, "diff cleared before commit, skipping")
		w.prevDiff = ""
		w.delayCounter = 0
		return
	}
	if err := w.git.Add(); err != nil {
		w.emit(EventError, fmt.Sprintf("git add: %s", err))
		return
	}
	if err := w.git.Commit(message); err != nil {
		w.emit(EventSkip, "pre-commit hooks failed, retrying next tick")
		w.prevDiff = ""
		w.delayCounter = 0
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
		fmt.Fprintf(os.Stderr, "warn: event channel full, dropped %s: %s\n", EventKindNames[kind], msg)
	}
}
