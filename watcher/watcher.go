package watcher

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"quill-commit/ai"
	"quill-commit/config"
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
	EventCommitError  // commit hook/script blocked the commit
	EventErrorExplain // AI explanation of a commit error
)

var EventKindNames = map[EventKind]string{
	EventCheck:        "EventCheck",
	EventSending:      "EventSending",
	EventDecision:     "EventDecision",
	EventCommit:       "EventCommit",
	EventAmend:        "EventAmend",
	EventForced:       "EventForced",
	EventError:        "EventError",
	EventSkip:         "EventSkip",
	EventDelay:        "EventDelay",
	EventInfo:         "EventInfo",
	EventCommitError:  "EventCommitError",
	EventErrorExplain: "EventErrorExplain",
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
	Detail  string // raw error text (EventCommitError) or AI fix (EventErrorExplain)
	Time    time.Time
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

// Watcher coordinates the scheduler, stabilizer, commit engine, context manager
// and event logger. It keeps the public API (Events, Cmds, Run, Close) and the
// test hook methods (tick, delayLoop, doAmend).
type Watcher struct {
	cfg     config.Config
	apiKey  string
	Events  chan Event
	Cmds    chan Cmd

	git gitOps
	ai  aiOps

	// Test-visible state. These are manipulated by watcher_test.go.
	prevDiff     string
	delayCounter int

	// blockedDiffHashes is the set of diff SHA-256 prefixes that previously failed
	// a commit (e.g. pre-commit hook). A diff whose hash is in this set is skipped
	// even if the diff briefly changed and came back. Cleared on successful commit.
	blockedDiffHashes map[string]struct{}

	sleepFn func(time.Duration)

	scheduler   *Scheduler
	stabilizer  *Stabilizer
	commitEng   *CommitEngine
	ctxMgr      *ContextManager
	eventLogger *EventLogger

	ctx    context.Context
	cancel context.CancelFunc

	logFile *os.File
}

// New creates a Watcher for the given configuration.
func New(ctx context.Context, cfg config.Config, apiKey string, repoRoot string) *Watcher {
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

	events := make(chan Event, 64)
	cmds := make(chan Cmd, 32)
	sleepFn := func(d time.Duration) {}
	if !isTest {
		sleepFn = time.Sleep
	}

	w := &Watcher{
		cfg:     cfg,
		apiKey:  apiKey,
		Events:  events,
		Cmds:    cmds,
		git:     realGit{},
		ai:      realAI{},
		sleepFn: sleepFn,
		ctx:     ctx,
		cancel:  cancel,
		logFile: logFile,
	}

	w.eventLogger = newEventLogger(ctx, events, logger)
	w.stabilizer = newStabilizer(cfg, w.git, w.sleepFn)
	w.commitEng = newCommitEngine(w.git, w.emit, w.emitDetail)
	w.ctxMgr = newContextManager(ctx, cfg, apiKey, repoRoot)

	// Scheduler ticks into w.tick and forwards other commands (e.g. Amend) back.
	w.scheduler = newScheduler(cfg, cmds, w.tick, w.handleCmd)

	return w
}

func (w *Watcher) emit(kind EventKind, msg string) {
	w.eventLogger.Emit(kind, msg)
}

func (w *Watcher) emitDetail(kind EventKind, msg, detail string) {
	w.eventLogger.EmitDetail(kind, msg, detail)
}

// Close stops the watcher and closes the log file.
func (w *Watcher) Close() {
	w.cancel()
	if w.logFile != nil {
		w.logFile.Close()
	}
}

// setGitAI is a test helper that swaps the git/ai adapters and propagates the
// git adapter to the sub-components that depend on it.
func (w *Watcher) setGitAI(g gitOps, a aiOps) {
	w.git = g
	w.stabilizer.git = g
	w.commitEng.git = g
	w.ai = a
}

// Run starts the watcher loop. It blocks until the watcher context is cancelled.
func (w *Watcher) Run() {
	w.scheduler.Run(w.ctx)
}

// handleCmd dispatches watcher commands. Currently only CmdAmend is handled here;
// pause/resume are processed by the scheduler.
func (w *Watcher) handleCmd(cmd Cmd) {
	switch cmd.Kind {
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
		SessionID:     w.ctxMgr.sessionID,
		ExplicitCache: false,
		Ctx:           w.ctx,
	}

	w.emit(EventSending, "asking model (amend)")
	decision, _, err := w.ai.Ask(req)
	if err != nil {
		w.emit(EventError, fmt.Sprintf("amend: ai error: %s", err))
		return
	}

	if err := w.commitEng.Amend(decision.Message, hasDiff); err == nil {
		w.prevDiff = ""
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

	// If this diff's content has already failed a commit (e.g. pre-commit hook),
	// skip silently even if the diff temporarily changed and came back.
	if _, blocked := w.blockedDiffHashes[diffHash(diff)]; blocked {
		return
	}

	if diff == "" {
		w.emit(EventSkip, "diff empty, waiting")
		w.prevDiff = ""
		w.delayCounter = 0
		return
	}

	stableDiff, newPrevDiff, ok := w.stabilizer.Stabilize(w.prevDiff, func() {
		w.emit(EventSkip, fmt.Sprintf("diff changed, re-checking in %s", formatDuration(w.cfg.Stabilize)))
	})
	if !ok {
		w.emit(EventSkip, "diff cleared during stabilization")
		w.prevDiff = ""
		return
	}
	w.prevDiff = newPrevDiff

	w.delayLoop(stableDiff)
}

// delayLoop asks the model and handles commit: false delays without recursion.
// After each sleep it re-checks the diff; if it changed, stabilization resets.
func (w *Watcher) delayLoop(stableDiff string) {
	for {
		req, dynErr := w.ctxMgr.BuildRequest(w.ctx, stableDiff)
		if dynErr != nil {
			w.emit(EventInfo, fmt.Sprintf("warn: context.BuildDynamic: %s", dynErr))
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

		w.ctxMgr.UpdateBudget(usage)

		if decision.Commit {
			if len(decision.Commits) > 1 {
				w.emit(EventDecision, fmt.Sprintf("model says split into %d commits", len(decision.Commits)))
				if err := w.commitEng.Split(decision.Commits); err != nil {
					w.blockDiff(stableDiff)
					w.explainCommitError(err.Error())
				} else {
					w.resetAfterCommit()
				}
				return
			}
			w.emit(EventDecision, fmt.Sprintf("model says commit: %s", decision.Message))
			if err := w.commitEng.Commit(decision.Message); err != nil {
				w.blockDiff(stableDiff)
				w.explainCommitError(err.Error())
			} else {
				w.resetAfterCommit()
			}
			return
		}

		w.delayCounter++
		if w.cfg.MaxDelays > 0 {
			w.emit(EventDecision, fmt.Sprintf("model says wait %ds (delay %d/%d)", decision.Delay, w.delayCounter, w.cfg.MaxDelays))
		} else {
			w.emit(EventDecision, fmt.Sprintf("model says wait %ds (delay %d)", decision.Delay, w.delayCounter))
		}

		if w.cfg.MaxDelays > 0 && w.delayCounter >= w.cfg.MaxDelays {
			w.emit(EventForced, "max delays reached, forcing commit")
			if err := w.commitEng.Commit("auto: forced commit"); err == nil {
				w.resetAfterCommit()
			}
			return
		}

		delay := decision.Delay
		if delay <= 0 {
			delay = 30
		}
		w.emit(EventDelay, fmt.Sprintf("sleeping %ds before retry", delay))
		w.sleepFn(time.Duration(delay) * time.Second)

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

func (w *Watcher) resetAfterCommit() {
	w.prevDiff = ""
	w.delayCounter = 0
	w.blockedDiffHashes = nil
}

// blockDiff records the hash of a diff that failed a commit so future ticks
// with the same content are skipped even if the diff briefly changed.
func (w *Watcher) blockDiff(diff string) {
	if w.blockedDiffHashes == nil {
		w.blockedDiffHashes = make(map[string]struct{})
	}
	w.blockedDiffHashes[diffHash(diff)] = struct{}{}
}

// diffHash returns a short hex fingerprint of a diff string.
func diffHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:8])
}

// explainCommitError asks the model to explain a commit failure (e.g. pre-commit hook)
// and emits EventErrorExplain with a short summary and fix suggestion.
func (w *Watcher) explainCommitError(rawErr string) {
	req := ai.Request{
		SystemPrompt:  ai.ExplainErrorPrompt,
		UserPrompt:    rawErr,
		Model:         w.cfg.Model,
		APIKey:        w.apiKey,
		SessionID:     w.ctxMgr.sessionID,
		ExplicitCache: false,
		Ctx:           w.ctx,
	}
	w.emit(EventSending, "asking model (explain error)")
	expl, err := ai.AskExplain(req)
	if err != nil {
		w.emit(EventInfo, "could not explain error: "+err.Error())
		return
	}
	w.emitDetail(EventErrorExplain, expl.Summary, expl.Fix)
}
