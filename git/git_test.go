package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func newTempRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "quill-commit-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	origDir, err := os.Getwd()
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("Chdir: %v", err)
	}
	if out, err := exec.Command("git", "init").CombinedOutput(); err != nil {
		os.Chdir(origDir) //nolint:errcheck
		os.RemoveAll(dir) //nolint:errcheck
		t.Fatalf("git init: %v\n%s", err, out)
	}
	exec.Command("git", "config", "user.email", "test@test").Run() //nolint:errcheck
	exec.Command("git", "config", "user.name", "test").Run()       //nolint:errcheck
	return dir, func() {
		os.Chdir(origDir)  //nolint:errcheck
		os.RemoveAll(dir)  //nolint:errcheck
	}
}

func TestRecentCommitsNZero(t *testing.T) {
	_, cleanup := newTempRepo(t)
	defer cleanup()
	got, err := RecentCommits(0)
	if err != nil {
		t.Fatalf("RecentCommits(0) err: %v", err)
	}
	if got != "" {
		t.Errorf("RecentCommits(0) = %q, want empty", got)
	}
	got, err = RecentCommits(-1)
	if err != nil {
		t.Fatalf("RecentCommits(-1) err: %v", err)
	}
	if got != "" {
		t.Errorf("RecentCommits(-1) = %q, want empty", got)
	}
}

func TestRecentCommitsUnborn(t *testing.T) {
	_, cleanup := newTempRepo(t)
	defer cleanup()
	got, err := RecentCommits(5)
	if err != nil {
		t.Fatalf("RecentCommits(5) err: %v", err)
	}
	if got != "" {
		t.Errorf("RecentCommits(5) on unborn branch = %q, want empty", got)
	}
}

func TestRecentCommitsThree(t *testing.T) {
	_, cleanup := newTempRepo(t)
	defer cleanup()
	for i := 0; i < 3; i++ {
		content := fmt.Sprintf("content %d", i)
		os.WriteFile("f.txt", []byte(content), 0644) //nolint:errcheck
		exec.Command("git", "add", "f.txt").Run()    //nolint:errcheck
		exec.Command("git", "commit", "-m", "commit").Run() //nolint:errcheck
	}
	got, err := RecentCommits(3)
	if err != nil {
		t.Fatalf("RecentCommits(3) err: %v", err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("RecentCommits(3) has %d lines, want 3", len(lines))
	}
	got, err = RecentCommits(10)
	if err != nil {
		t.Fatalf("RecentCommits(10) err: %v", err)
	}
	lines = strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("RecentCommits(10) has %d lines, want 3 (only 3 exist)", len(lines))
	}
}

func TestStatusShortClean(t *testing.T) {
	_, cleanup := newTempRepo(t)
	defer cleanup()
	os.WriteFile("a.txt", []byte("x"), 0644) //nolint:errcheck
	exec.Command("git", "add", "a.txt").Run() //nolint:errcheck
	exec.Command("git", "commit", "-m", "init").Run() //nolint:errcheck
	got, err := StatusShort()
	if err != nil {
		t.Fatalf("StatusShort() err: %v", err)
	}
	if got != "" {
		t.Errorf("StatusShort() on clean tree = %q, want empty", got)
	}
}

func TestStatusShortDirty(t *testing.T) {
	_, cleanup := newTempRepo(t)
	defer cleanup()
	os.WriteFile("a.txt", []byte("x"), 0644) //nolint:errcheck
	exec.Command("git", "add", "a.txt").Run() //nolint:errcheck
	exec.Command("git", "commit", "-m", "init").Run() //nolint:errcheck
	os.WriteFile("b.txt", []byte("y"), 0644) //nolint:errcheck
	got, err := StatusShort()
	if err != nil {
		t.Fatalf("StatusShort() err: %v", err)
	}
	if got == "" {
		t.Fatal("StatusShort() on dirty tree = empty, want non-empty")
	}
}

func TestLsFiles(t *testing.T) {
	_, cleanup := newTempRepo(t)
	defer cleanup()
	os.MkdirAll("sub/deep", 0755)             //nolint:errcheck
	os.WriteFile("top.txt", []byte("a"), 0644)           //nolint:errcheck
	os.WriteFile("sub/mid.txt", []byte("b"), 0644)       //nolint:errcheck
	os.WriteFile("sub/deep/bot.txt", []byte("c"), 0644)  //nolint:errcheck
	exec.Command("git", "add", "-A").Run()                //nolint:errcheck
	exec.Command("git", "commit", "-m", "init").Run()     //nolint:errcheck
	got, err := LsFiles()
	if err != nil {
		t.Fatalf("LsFiles() err: %v", err)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("LsFiles() has %d lines, want 3", len(lines))
	}
	if lines[0] != "sub/deep/bot.txt" {
		t.Errorf("sorted[0] = %q, want %q", lines[0], "sub/deep/bot.txt")
	}
	if lines[1] != "sub/mid.txt" {
		t.Errorf("sorted[1] = %q, want %q", lines[1], "sub/mid.txt")
	}
	if lines[2] != "top.txt" {
		t.Errorf("sorted[2] = %q, want %q", lines[2], "top.txt")
	}
}

func TestIsRepoTrue(t *testing.T) {
	if !IsRepo() {
		t.Error("expected true in git repo")
	}
}

func TestDiffEmpty(t *testing.T) {
	diff, err := Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	if diff == "" {
		t.Log("diff is empty (clean working tree)")
	}
}

func TestAddNoError(t *testing.T) {
	if err := Add(); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}
}
