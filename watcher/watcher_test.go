package watcher

import (
	"errors"
	"testing"

	"quill-commit/ai"
	"quill-commit/config"
)

// --- fakes ---

type fakeGit struct {
	diffs   []string
	diffIdx int
	added   bool
	commits []string
	diffErr error
}

func (f *fakeGit) Diff() (string, error) {
	if f.diffErr != nil {
		return "", f.diffErr
	}
	if f.diffIdx >= len(f.diffs) {
		return f.diffs[len(f.diffs)-1], nil
	}
	d := f.diffs[f.diffIdx]
	f.diffIdx++
	return d, nil
}

func (f *fakeGit) Add() error                  { f.added = true; return nil }
func (f *fakeGit) Commit(msg string) error      { f.commits = append(f.commits, msg); return nil }

type fakeAI struct {
	responses []ai.Decision
	respIdx   int
	err       error
	AskFunc   func(req ai.Request) (ai.Decision, ai.Usage, error)
}

func (f *fakeAI) Ask(req ai.Request) (ai.Decision, ai.Usage, error) {
	if f.AskFunc != nil {
		return f.AskFunc(req)
	}
	if f.err != nil {
		return ai.Decision{}, ai.Usage{}, f.err
	}
	if len(f.responses) == 0 {
		return ai.Decision{}, ai.Usage{}, nil
	}
	var d ai.Decision
	if f.respIdx >= len(f.responses) {
		d = f.responses[len(f.responses)-1]
	} else {
		d = f.responses[f.respIdx]
		f.respIdx++
	}
	return d, ai.Usage{}, nil
}

func newTestWatcher(g *fakeGit, a *fakeAI) *Watcher {
	cfg := config.Config{Interval: 10, Stabilize: 0, MaxDelays: 3, Model: "test"}
	w := New(cfg, "key", "")
	w.git = g
	w.ai = a
	return w
}

func collectEvents(w *Watcher) []Event {
	var evs []Event
	for {
		select {
		case e := <-w.Events:
			evs = append(evs, e)
		default:
			return evs
		}
	}
}

func kindsOf(evs []Event) []EventKind {
	kinds := make([]EventKind, len(evs))
	for i, e := range evs {
		kinds[i] = e.Kind
	}
	return kinds
}

// --- tests ---

func TestTick_EmptyDiff_Skips(t *testing.T) {
	g := &fakeGit{diffs: []string{""}}
	w := newTestWatcher(g, &fakeAI{})
	w.tick()

	evs := collectEvents(w)
	kinds := kindsOf(evs)
	if len(kinds) < 2 || kinds[0] != EventCheck || kinds[1] != EventSkip {
		t.Fatalf("expected Check+Skip, got %v", kinds)
	}
	if len(g.commits) != 0 {
		t.Fatal("should not commit on empty diff")
	}
}

// Stabilization loop: diff changes once during re-check, then settles — commits with final diff.
func TestTick_DiffChangingThenStable_Commits(t *testing.T) {
	// Diff sequence: first check=diff-a, re-check=diff-b, re-check=diff-b (stable)
	g := &fakeGit{diffs: []string{"diff-a", "diff-b", "diff-b"}}
	a := &fakeAI{responses: []ai.Decision{{Commit: true, Message: "feat: done"}}}
	w := newTestWatcher(g, a)

	w.tick()

	evs := collectEvents(w)
	var committed bool
	for _, e := range evs {
		if e.Kind == EventCommit {
			committed = true
		}
	}
	if !committed {
		t.Fatal("expected commit after stabilization")
	}
	if len(g.commits) != 1 || g.commits[0] != "feat: done" {
		t.Fatalf("unexpected commits: %v", g.commits)
	}
}

// Already-stable diff (prevDiff pre-set) commits on the first tick without looping.
func TestTick_StableDiff_Commits(t *testing.T) {
	g := &fakeGit{diffs: []string{"diff-a"}}
	a := &fakeAI{responses: []ai.Decision{{Commit: true, Message: "feat: done"}}}
	w := newTestWatcher(g, a)
	w.prevDiff = "diff-a" // pre-stable: no re-check needed

	w.tick()

	evs := collectEvents(w)
	var committed bool
	for _, e := range evs {
		if e.Kind == EventCommit {
			committed = true
		}
	}
	if !committed {
		t.Fatal("expected commit on already-stable diff")
	}
	if len(g.commits) != 1 || g.commits[0] != "feat: done" {
		t.Fatalf("unexpected commits: %v", g.commits)
	}
	if w.prevDiff != "" || w.delayCounter != 0 {
		t.Fatal("state not reset after commit")
	}
}

func TestDelayLoop_MaxDelays_ForcesCommit(t *testing.T) {
	// diff stays same throughout all Diff() calls
	g := &fakeGit{diffs: []string{"diff-x"}}
	a := &fakeAI{responses: []ai.Decision{
		{Commit: false, Delay: 0},
		{Commit: false, Delay: 0},
		{Commit: false, Delay: 0},
	}}
	w := newTestWatcher(g, a)
	w.prevDiff = "diff-x" // already stable

	w.delayLoop("diff-x")

	if len(g.commits) != 1 || g.commits[0] != "auto: forced commit" {
		t.Fatalf("expected forced commit, got %v", g.commits)
	}

	var forced bool
	for _, e := range collectEvents(w) {
		if e.Kind == EventForced {
			forced = true
		}
	}
	if !forced {
		t.Fatal("expected EventForced")
	}
}

func TestDelayLoop_NetworkError_ResetsCounter(t *testing.T) {
	g := &fakeGit{diffs: []string{"diff-x"}}
	a := &fakeAI{err: errors.New("connection refused")}
	w := newTestWatcher(g, a)
	w.delayCounter = 2 // pre-elevated counter

	w.delayLoop("diff-x")

	if w.delayCounter != 0 {
		t.Fatalf("expected counter reset to 0 after network error, got %d", w.delayCounter)
	}
	var hasErr bool
	for _, e := range collectEvents(w) {
		if e.Kind == EventError {
			hasErr = true
		}
	}
	if !hasErr {
		t.Fatal("expected EventError on network failure")
	}
}

func TestDelayLoop_DiffChangedDuringSleep_ResetsStabilization(t *testing.T) {
	// First Diff() call (initial) returns diff-x; second (after "sleep") returns diff-y
	g := &fakeGit{diffs: []string{"diff-x", "diff-y"}}
	a := &fakeAI{responses: []ai.Decision{{Commit: false, Delay: 0}}}
	w := newTestWatcher(g, a)

	w.delayLoop("diff-x")

	if len(g.commits) != 0 {
		t.Fatal("should not commit when diff changed during delay")
	}
	if w.prevDiff != "diff-y" {
		t.Fatalf("expected prevDiff=diff-y, got %q", w.prevDiff)
	}
	if w.delayCounter != 0 {
		t.Fatalf("expected counter reset, got %d", w.delayCounter)
	}
}

func TestWatcherSessionID(t *testing.T) {
	cfg := config.Config{
		Interval:       10,
		Stabilize:      0,
		MaxDelays:      3,
		Model:          "test",
		IncludeContext: true,
		SessionID:      "explicit-id",
	}

	oldCap := ai.CacheCapabilityFn
	ai.CacheCapabilityFn = func(model, apiKey string) (bool, error) {
		return true, nil
	}
	defer func() { ai.CacheCapabilityFn = oldCap }()

	var lastReq ai.Request
	a := &fakeAI{
		responses: []ai.Decision{{Commit: true, Message: "feat: commit"}},
		AskFunc: func(req ai.Request) (ai.Decision, ai.Usage, error) {
			lastReq = req
			return ai.Decision{Commit: true, Message: "feat: commit"}, ai.Usage{}, nil
		},
	}
	g := &fakeGit{diffs: []string{"diff-x"}}

	w := New(cfg, "key", t.TempDir())
	w.git = g
	w.ai = a
	w.prevDiff = "diff-x"

	w.delayLoop("diff-x")

	if lastReq.SessionID != "explicit-id" {
		t.Errorf("expected Request.SessionID 'explicit-id', got %q", lastReq.SessionID)
	}
}
