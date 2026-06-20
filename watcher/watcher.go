package watcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"quill-commit/ai"
	"quill-commit/config"
	gitcontext "quill-commit/context"
	"quill-commit/git"
)

type EventKind int

const (
	EventCheck EventKind = iota
	EventSending
	EventDecision
	EventCommit
	EventAmend
	EventForced
	EventError
	EventSkip
	EventDelay
	EventInfo
)

var EventKindNames = map[EventKind]string{
	EventCheck:    "EventCheck",
	EventSending:  "EventSending",
	EventDecision: "EventDecision",
	EventCommit:   "EventCommit",
	EventAmend:    "EventAmend",
	EventForced:   "EventForced",
	EventError:    "EventError",
	EventSkip:     "EventSkip",
	EventDelay:    "EventDelay",
	EventInfo:     "EventInfo",
}

type CmdKind int

const (
	CmdPause CmdKind = iota
	CmdResume
	CmdAmend
)

type Cmd struct {
	Kind CmdKind
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
	AddPaths(paths []string) error
	Commit(message string) error
	HeadMessage() (string, error)
	AmendCommit(message string) error
}

type aiOps interface {
	Ask(req ai.Request) (ai.Decision, ai.Usage, error)
}

type realGit struct{}

func (realGit) Diff() (string, error)           { return git.Diff() }
func (realGit) Add() error                       { return git.Add() }
func (realGit) AddPaths(paths []string) error    { return git.AddPaths(paths) }
func (realGit) Commit(message string) error      { return git.Commit(message) }
func (realGit) HeadMessage() (string, error)     { return git.HeadMessage() }
func (realGit) AmendCommit(message string) error { return git.AmendCommit(message) }

type realAI struct{}

func (realAI) Ask(req ai.Request) (ai.Decision, ai.Usage, error) {
	return ai.Ask(req)
}

type Watcher struct {
	cfg    config.Config
	apiKey string
	Events chan Event
	Cmds   chan Cmd

	git gitOps
	ai  aiOps

	paused atomic.Bool

	prevDiff     string
	delayCounter int

	// Context fields
	static        gitcontext.Static
	staticBudget  int
	fullBudget    int
	sessionID     string
	explicitCache bool
	cacheMisses   int

	sleepFn       func(time.Duration)
	repoRoot      string

	ctx           context.Context
	cancel        context.CancelFunc

	logFile       *os.File
	logger        *slog.Logger
}

func New(ctx context.Context, cfg config.Config, apiKey string, repoRoot string) *Watcher {
	var static gitcontext.Static
	var sessionID string
	var explicitCache bool
	var staticBudget, fullBudget int

	if cfg.IncludeContext {
		var err error
		static, err = gitcontext.BuildStatic(repoRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warn: context.BuildStatic:", err)
		}

		if cfg.SessionID != "" {
			sessionID = cfg.SessionID
		} else {
			b := make([]byte, 16)
			if _, err := rand.Read(b); err != nil {
				fmt.Fprintln(os.Stderr, "warn: generate session_id:", err)
				now := time.Now().UnixNano()
				for i := 0; i < 8; i++ {
					b[i] = byte(now >> (i * 8))
				}
				pid := os.Getpid()
				for i := 0; i < 4; i++ {
					b[8+i] = byte(pid >> (i * 8))
				}
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

	ctx, cancel := context.WithCancel(ctx)

	var logFile *os.File
	var logger *slog.Logger
	isTest := flag.Lookup("test.v") != nil

	if repoRoot != "" && !isTest {
		logPath := filepath.Join(repoRoot, "log.txt")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warn: could not open log file:", err)
		} else {
			logFile = f
			logger = slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))
		}
	}

	w := &Watcher{
		cfg:           cfg,
		apiKey:        apiKey,
		Events:        make(chan Event, 64),
		Cmds:          make(chan Cmd, 32),
		git:           realGit{},
		ai:            realAI{},
		static:        static,
		staticBudget:  staticBudget,
		fullBudget:    fullBudget,
		sessionID:     sessionID,
		explicitCache: explicitCache,
		repoRoot:      repoRoot,
		ctx:           ctx,
		cancel:        cancel,
		logFile:       logFile,
		logger:        logger,
	}
	w.sleepFn = w.sleep
	if isTest {
		w.sleepFn = func(d time.Duration) {}
	}
	return w
}

func (w *Watcher) Close() {
	w.cancel()
	if w.logFile != nil {
		w.logFile.Close()
	}
}

func (w *Watcher) sleep(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-w.ctx.Done():
	case <-timer.C:
	case cmd := <-w.Cmds:
		w.handleCmd(cmd)
	}
}

func (w *Watcher) Run() {
	interval := w.cfg.Interval
	if interval <= 0 {
		interval = config.DefaultInterval
	}
	ticker := time.NewTicker(time.Duration(interval * float64(time.Minute)))
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if !w.paused.Load() {
				w.tick()
			}
		case cmd := <-w.Cmds:
			w.handleCmd(cmd)
		}
	}
}

func (w *Watcher) handleCmd(cmd Cmd) {
	switch cmd.Kind {
	case CmdPause:
		w.paused.Store(true)
	case CmdResume:
		w.paused.Store(false)
	case CmdAmend:
		w.doAmend()
	}
}

func (w *Watcher) doAmend() {
	diff, err := w.git.Diff()
	if err != nil {
		w.emit(EventError, fmt.Sprintf("amend: git diff: %s", err))
		return
	}

	original, err := w.git.HeadMessage()
	if err != nil {
		w.emit(EventError, fmt.Sprintf("amend: git log: %s", err))
		return
	}

	if diff == "" && original == "" {
		w.emit(EventInfo, "amend: nothing to amend")
		return
	}

	var (
		systemPrompt string
		userPrompt   string
		hasDiff      bool
	)

	if diff == "" {
		systemPrompt = ai.AmendRewritePrompt
		userPrompt = "Original commit message:\n" + original
	} else {
		systemPrompt = ai.AmendBasePrompt
		userPrompt = "Original commit message:\n" + original + "\n\nAdditional diff:\n" + diff
		hasDiff = true
	}

	req := ai.Request{
		SystemPrompt:  systemPrompt,
		UserPrompt:    userPrompt,
		Model:         w.cfg.Model,
		APIKey:        w.apiKey,
		SessionID:     w.sessionID,
		ExplicitCache: false,
		Ctx:           w.ctx,
	}

	w.emit(EventSending, "asking model (amend)")
	decision, _, err := w.ai.Ask(req)
	if err != nil {
		w.emit(EventError, fmt.Sprintf("amend: ai error: %s", err))
		return
	}

	if hasDiff {
		if err := w.git.Add(); err != nil {
			w.emit(EventError, fmt.Sprintf("amend: git add: %s", err))
			return
		}
	}
	if err := w.git.AmendCommit(decision.Message); err != nil {
		w.emit(EventError, fmt.Sprintf("amend: git commit --amend: %s", err))
		return
	}
	w.emit(EventAmend, decision.Message)
	w.prevDiff = ""
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
		w.sleepFn(2 * time.Second)
		return
	}

	for diff != w.prevDiff {
		w.emit(EventSkip, fmt.Sprintf("diff changed, re-checking in %s", formatDuration(w.cfg.Stabilize)))
		w.sleepFn(2 * time.Second)
		w.prevDiff = diff
		w.sleepFn(time.Duration(w.cfg.Stabilize * float64(time.Minute)))
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
			dyn, dynErr := gitcontext.BuildDynamic(w.cfg.RecentCommitsCount)
			if dynErr != nil {
				w.emit(EventInfo, fmt.Sprintf("warn: context.BuildDynamic: %s", dynErr))
			}
			sysPrompt = ai.BasePrompt + "\n\n---\n\n" + gitcontext.RenderSystem(w.static, w.staticBudget)
			userPrompt = gitcontext.RenderUser(dyn, stableDiff)
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
			Ctx:           w.ctx,
		}

		w.emit(EventSending, "asking model")
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
				}
			} else {
				w.cacheMisses++
				if w.cacheMisses >= 3 && w.staticBudget > 800 {
					w.staticBudget = 800
					if w.staticBudget > w.fullBudget {
						w.staticBudget = w.fullBudget
					}
					w.cacheMisses = 0
				}
			}
		}

		if decision.Commit {
			if len(decision.Commits) > 1 {
				w.emit(EventDecision, fmt.Sprintf("model says split into %d commits", len(decision.Commits)))
				w.doSplit(decision.Commits)
				return
			}
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

		delay := decision.Delay
		if delay <= 0 {
			delay = 1
		}
		w.emit(EventDelay, fmt.Sprintf("sleeping %dm before retry", delay))
		w.sleepFn(time.Duration(delay) * time.Minute)

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

// doSplit commits each group sequentially, staging only that group's files.
// Any files the model omitted are swept into a final commit so nothing is left behind.
func (w *Watcher) doSplit(groups []ai.CommitGroup) {
	committed := false
	for _, g := range groups {
		var cleanFiles []string
		for _, f := range g.Files {
			f = strings.TrimSpace(f)
			if f != "" {
				cleanFiles = append(cleanFiles, f)
			}
		}
		if len(cleanFiles) == 0 || g.Message == "" {
			continue
		}
		if err := w.git.AddPaths(cleanFiles); err != nil {
			w.emit(EventError, fmt.Sprintf("split: git add %v: %s", cleanFiles, err))
			continue
		}
		if err := w.git.Commit(g.Message); err != nil {
			w.emit(EventSkip, "split: pre-commit hooks failed, retrying next tick")
			w.prevDiff = ""
			w.delayCounter = 0
			return
		}
		w.emit(EventCommit, g.Message)
		committed = true
	}

	// Sweep any leftover changes the model didn't assign to a group.
	leftover, err := w.git.Diff()
	if err == nil && leftover != "" {
		if err := w.git.Add(); err == nil {
			if err := w.git.Commit("chore: commit remaining changes"); err == nil {
				w.emit(EventCommit, "chore: commit remaining changes")
				committed = true
			}
		}
	}

	if !committed {
		// Nothing landed — fall back so the cycle doesn't silently lose work.
		w.doCommit("auto: fallback commit")
		return
	}
	w.prevDiff = ""
	w.delayCounter = 0
}

func (w *Watcher) doCommit(message string) {
	diff, err := w.git.Diff()
	if err != nil {
		w.emit(EventError, fmt.Sprintf("git diff before commit: %s", err))
		w.prevDiff = ""
		w.delayCounter = 0
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
		w.prevDiff = ""
		w.delayCounter = 0
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
	name, ok := EventKindNames[kind]
	if !ok {
		name = fmt.Sprintf("UnknownEvent(%d)", kind)
	}

	if w.logger != nil {
		var level slog.Level
		switch kind {
		case EventError:
			level = slog.LevelError
		case EventForced:
			level = slog.LevelWarn
		case EventCheck, EventSkip:
			level = slog.LevelDebug
		default:
			level = slog.LevelInfo
		}
		w.logger.Log(w.ctx, level, msg, slog.String("event", name))
	}

	select {
	case w.Events <- newEvent(kind, msg):
	default:
		fmt.Fprintf(os.Stderr, "warn: event channel full, dropped %s: %s\n", name, msg)
	}
}
