package watcher

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"quill-commit/ai"
	"quill-commit/config"
	"quill-commit/context"
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
	EventInfo
)

var EventKindNames = map[EventKind]string{
	EventCheck:    "EventCheck",
	EventDecision: "EventDecision",
	EventCommit:   "EventCommit",
	EventForced:   "EventForced",
	EventError:    "EventError",
	EventSkip:     "EventSkip",
	EventDelay:    "EventDelay",
	EventInfo:     "EventInfo",
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
	Ask(req ai.Request) (ai.Decision, ai.Usage, error)
}

type realGit struct{}

func (realGit) Diff() (string, error)       { return git.Diff() }
func (realGit) Add() error                  { return git.Add() }
func (realGit) Commit(message string) error { return git.Commit(message) }

type realAI struct{}

func (realAI) Ask(req ai.Request) (ai.Decision, ai.Usage, error) {
	return ai.Ask(req)
}

type Watcher struct {
	cfg    config.Config
	apiKey string
	Events chan Event

	git gitOps
	ai  aiOps

	prevDiff     string
	delayCounter int

	// Context fields
	static        context.Static
	staticBudget  int
	fullBudget    int
	sessionID     string
	explicitCache bool
	cacheMisses   int
}

func New(cfg config.Config, apiKey string, repoRoot string) *Watcher {
	var static context.Static
	var sessionID string
	var explicitCache bool
	var staticBudget, fullBudget int

	if cfg.IncludeContext {
		var err error
		static, err = context.BuildStatic(repoRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warn: context.BuildStatic:", err)
		}

		if cfg.SessionID != "" {
			sessionID = cfg.SessionID
		} else {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				fmt.Fprintln(os.Stderr, "warn: generate session_id:", err)
			}
			sessionID = hex.EncodeToString(b)
		}

		var capErr error
		explicitCache, capErr = ai.CacheCapabilityFn(cfg.Model, apiKey)
		if capErr != nil {
			fmt.Fprintln(os.Stderr, "warn: CacheCapability:", capErr)
			explicitCache = false
		}

		staticBudget = cfg.ContextBudget
		fullBudget = cfg.ContextBudget
	}

	return &Watcher{
		cfg:           cfg,
		apiKey:        apiKey,
		Events:        make(chan Event, 64),
		git:           realGit{},
		ai:            realAI{},
		static:        static,
		staticBudget:  staticBudget,
		fullBudget:    fullBudget,
		sessionID:     sessionID,
		explicitCache: explicitCache,
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
		w.delayCounter = 0
		return
	}

	w.emit(EventCheck, "checking diff")

	if diff == "" {
		w.emit(EventSkip, "diff empty, waiting")
		w.prevDiff = ""
		w.delayCounter = 0
		time.Sleep(2 * time.Second)
		return
	}

	for diff != w.prevDiff {
		w.emit(EventSkip, fmt.Sprintf("diff changed, re-checking in %s", formatDuration(w.cfg.Stabilize)))
		time.Sleep(2 * time.Second)
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
		var sysPrompt string
		var userPrompt string

		if w.cfg.IncludeContext {
			dyn, dynErr := context.BuildDynamic(w.cfg.RecentCommitsCount)
			if dynErr != nil {
				w.emit(EventInfo, fmt.Sprintf("warn: context.BuildDynamic: %s", dynErr))
			}
			sysPrompt = ai.BasePrompt + "\n\n---\n\n" + context.RenderSystem(w.static, w.staticBudget)
			userPrompt = context.RenderUser(dyn, stableDiff)
		} else {
			sysPrompt = ai.BasePrompt
			userPrompt = stableDiff
		}

		req := ai.Request{
			SystemPrompt:  sysPrompt,
			UserPrompt:    userPrompt,
			Model:         w.cfg.Model,
			APIKey:        w.apiKey,
			SessionID:     w.sessionID,
			ExplicitCache: w.explicitCache,
		}

		decision, usage, err := w.ai.Ask(req)
		if err != nil {
			// network error: do not count toward delay counter; reset so next
			// stabilization cycle starts clean
			w.emit(EventError, fmt.Sprintf("ai error (skipping): %s", err))
			w.delayCounter = 0
			return
		}

		if w.cfg.IncludeContext {
			if usage.CachedTokens > 0 {
				if w.cacheMisses > 0 || w.staticBudget < w.fullBudget {
					w.cacheMisses = 0
					w.staticBudget = w.fullBudget
					w.emit(EventInfo, "cache: recovered full budget")
				}
				w.emit(EventInfo, fmt.Sprintf("cache: hit %d tok", usage.CachedTokens))
			} else {
				w.cacheMisses++
				w.emit(EventInfo, fmt.Sprintf("cache: miss (%d)", w.cacheMisses))
				if w.cacheMisses >= 3 && w.staticBudget > 800 {
					w.staticBudget = 800
					w.cacheMisses = 0
					w.emit(EventInfo, "cache: budget shrunk to 800 chars")
				}
			}
		}

		if decision.Commit {
			w.emit(EventDecision, fmt.Sprintf("model says commit: %s", decision.Message))
			w.doCommit(decision.Message)
			return
		}

		w.delayCounter++
		if w.cfg.MaxDelays > 0 {
			w.emit(EventDecision, fmt.Sprintf("model says wait %dm (delay %d/%d)", decision.Delay, w.delayCounter, w.cfg.MaxDelays))
		} else {
			w.emit(EventDecision, fmt.Sprintf("model says wait %dm (delay %d)", decision.Delay, w.delayCounter))
		}

		if w.cfg.MaxDelays > 0 && w.delayCounter >= w.cfg.MaxDelays {
			w.emit(EventForced, "max delays reached, forcing commit")
			w.doCommit("auto: forced commit")
			return
		}

		w.emit(EventDelay, fmt.Sprintf("sleeping %dm before retry", decision.Delay))
		time.Sleep(time.Duration(decision.Delay) * time.Minute)

		currentDiff, err := w.git.Diff()
		if err != nil {
			w.emit(EventError, fmt.Sprintf("git diff after delay: %s", err))
			w.delayCounter = 0
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
