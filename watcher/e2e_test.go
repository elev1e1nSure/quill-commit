package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"quill-commit/ai"
	"quill-commit/config"
)

// e2eRepo initialises a temporary git repository, sets the test's working
// directory to it, and returns the repo root path.
func e2eRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)

	hooksDir := t.TempDir() // empty dir → no hooks run
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "ci@test"},
		{"git", "config", "user.name", "CI"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "config", "core.hooksPath", hooksDir},
	} {
		e2eRun(t, args[0], args[1:]...)
	}

	// Initial commit so HEAD is valid and git diff has a baseline.
	e2eWrite(t, filepath.Join(dir, ".gitkeep"), "")
	e2eRun(t, "git", "add", ".gitkeep")
	e2eRun(t, "git", "commit", "-m", "chore: init")
	return dir
}

func e2eRun(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func e2eWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func e2eHeadMsg(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "log", "--pretty=format:%s", "-1").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// e2eHeadFiles returns the list of files changed in HEAD.
func e2eHeadFiles(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "show", "--name-only", "--format=", "HEAD").Output()
	if err != nil {
		t.Fatalf("git show HEAD: %v", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			files = append(files, line)
		}
	}
	return files
}

func e2eContains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func e2eWatcher(t *testing.T, dir string, a aiOps) *Watcher {
	t.Helper()
	cfg := config.Config{
		Interval:  0.1,
		Stabilize: 0,
		MaxDelays: 0,
		Model:     "test",
		// IncludeContext: false (zero value) — no HTTP probes to OpenRouter
	}
	w := New(context.Background(), cfg, "test-key", dir)
	w.ai = a
	return w
}

// TestE2E_CommitCycle verifies the full path: file added → diff detected →
// AI approves → real git commit created with the generated message.
func TestE2E_CommitCycle(t *testing.T) {
	dir := e2eRepo(t)
	e2eWrite(t, filepath.Join(dir, "main.go"), "package main\n")

	a := &fakeAI{responses: []ai.Decision{{Commit: true, Message: "feat: add main"}}}
	w := e2eWatcher(t, dir, a)
	w.tick()

	if msg := e2eHeadMsg(t); msg != "feat: add main" {
		t.Fatalf("expected %q, got %q", "feat: add main", msg)
	}
	files := e2eHeadFiles(t)
	if !e2eContains(files, "main.go") {
		t.Fatalf("main.go not in HEAD: %v", files)
	}
}

// TestE2E_SecretFileNeverCommitted verifies that a .env file with a known
// secret signature is excluded from the commit even when a clean file is
// present in the same working tree.
func TestE2E_SecretFileNeverCommitted(t *testing.T) {
	dir := e2eRepo(t)
	e2eWrite(t, filepath.Join(dir, "main.go"), "package main\n")
	// Content scan pattern: sk-or-v1- triggers exclusion.
	e2eWrite(t, filepath.Join(dir, ".env"), "QUILL_API_KEY=sk-or-v1-abc123\n")

	a := &fakeAI{responses: []ai.Decision{{Commit: true, Message: "feat: add main"}}}
	w := e2eWatcher(t, dir, a)
	w.tick()

	files := e2eHeadFiles(t)
	if !e2eContains(files, "main.go") {
		t.Fatalf("main.go should be in HEAD: %v", files)
	}
	if e2eContains(files, ".env") {
		t.Fatalf(".env must not be in HEAD: %v", files)
	}
}

// TestE2E_QuarantineSkipsAI verifies that when only secret files are present
// in the working tree, the watcher silently skips without calling the model.
func TestE2E_QuarantineSkipsAI(t *testing.T) {
	dir := e2eRepo(t)
	// Only a file that is both path-blocked (.env) and content-blocked.
	e2eWrite(t, filepath.Join(dir, ".env"), "API_KEY=sk-or-v1-abc123\n")

	aiCallCount := 0
	a := &fakeAI{AskFunc: func(_ ai.Request) (ai.Decision, ai.Usage, error) {
		aiCallCount++
		return ai.Decision{Commit: true, Message: "should not reach git"}, ai.Usage{}, nil
	}}
	w := e2eWatcher(t, dir, a)
	w.tick()

	if aiCallCount > 0 {
		t.Fatalf("AI must not be called during quarantine, was called %d time(s)", aiCallCount)
	}
	if msg := e2eHeadMsg(t); msg == "should not reach git" {
		t.Fatal("quarantine should have prevented any commit")
	}

	var sawQuarantine bool
	for _, e := range collectEvents(w) {
		if e.Kind == EventInfo && strings.HasPrefix(e.Message, "quarantine:") {
			sawQuarantine = true
		}
	}
	if !sawQuarantine {
		t.Fatal("expected quarantine EventInfo event")
	}
}
