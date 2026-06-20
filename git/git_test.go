package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTempRepo(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "quill-commit-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}

	// Backup and unset git env vars that can interfere when tests run inside hooks
	envBackup := make(map[string]string)
	gitEnvVars := []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_WORK_TREE", "GIT_PREFIX"}
	for _, env := range gitEnvVars {
		if val, exists := os.LookupEnv(env); exists {
			envBackup[env] = val
			os.Unsetenv(env)
		}
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
		// Restore env vars
		for env, val := range envBackup {
			os.Setenv(env, val)
		}
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustRun(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestDiffExNameOnlyFilter(t *testing.T) {
	dir, cleanup := newTempRepo(t)
	defer cleanup()

	// Create a secret file and a normal file.
	mustWriteFile(t, filepath.Join(dir, ".env"), "SECRET=123\n")
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\n")
	mustRun(t, "git", "add", ".")
	mustRun(t, "git", "commit", "-m", "initial")

	// Modify both.
	mustWriteFile(t, filepath.Join(dir, ".env"), "SECRET=456\n")
	mustWriteFile(t, filepath.Join(dir, "main.go"), "package main\n\nfunc main() {}\n")

	res, err := DiffEx(dir)
	if err != nil {
		t.Fatalf("DiffEx: %v", err)
	}

	// .env should be excluded.
	foundEnv := false
	for _, f := range res.ExcludedFiles {
		if f == ".env" {
			foundEnv = true
			break
		}
	}
	if !foundEnv {
		t.Errorf("expected .env in ExcludedFiles, got %v", res.ExcludedFiles)
	}

	// main.go should be included.
	foundMain := false
	for _, f := range res.IncludedFiles {
		if f == "main.go" {
			foundMain = true
			break
		}
	}
	if !foundMain {
		t.Errorf("expected main.go in IncludedFiles, got %v", res.IncludedFiles)
	}

	// .env should not appear in the diff string.
	if strings.Contains(res.Diff, ".env") {
		t.Error("expected diff to not contain .env")
	}
}

func TestDiffExTrackedSecretExcluded(t *testing.T) {
	dir, cleanup := newTempRepo(t)
	defer cleanup()

	mustWriteFile(t, filepath.Join(dir, "config.go"), "package main\n")
	mustRun(t, "git", "add", ".")
	mustRun(t, "git", "commit", "-m", "initial")

	// Add a secret line to a tracked file.
	mustWriteFile(t, filepath.Join(dir, "config.go"), "package main\n\n// key: sk-or-v1-abc123def456ghi789jkl012mno345pqr678stu\n")

	res, err := DiffEx(dir)
	if err != nil {
		t.Fatalf("DiffEx: %v", err)
	}

	// config.go should be excluded because its added line contains a secret.
	found := false
	for _, f := range res.ExcludedFiles {
		if f == "config.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected config.go in ExcludedFiles, got %v", res.ExcludedFiles)
	}

	// config.go should not be in IncludedFiles.
	for _, f := range res.IncludedFiles {
		if f == "config.go" {
			t.Errorf("expected config.go to NOT be in IncludedFiles")
		}
	}

	// The secret should not appear in the diff string.
	if strings.Contains(res.Diff, "sk-or-v1-") {
		t.Error("expected diff to not contain the secret token")
	}
}

func TestDiffExUntrackedSecret(t *testing.T) {
	dir, cleanup := newTempRepo(t)
	defer cleanup()

	// Make an initial commit so HEAD exists.
	mustWriteFile(t, filepath.Join(dir, "a.go"), "package main\n")
	mustRun(t, "git", "add", ".")
	mustRun(t, "git", "commit", "-m", "initial")

	// Create an untracked file with a secret.
	mustWriteFile(t, filepath.Join(dir, "tokens.txt"), "api_key=sk-or-v1-abc123def456ghi789jkl012mno345pqr678stu\n")

	res, err := DiffEx(dir)
	if err != nil {
		t.Fatalf("DiffEx: %v", err)
	}

	found := false
	for _, f := range res.ExcludedFiles {
		if f == "tokens.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tokens.txt in ExcludedFiles, got %v", res.ExcludedFiles)
	}

	// The secret should not appear in the diff string.
	if strings.Contains(res.Diff, "sk-or-v1-") {
		t.Error("expected diff to not contain the secret token")
	}
}

func TestDiffExBackwardCompatible(t *testing.T) {
	dir, cleanup := newTempRepo(t)
	defer cleanup()

	mustWriteFile(t, filepath.Join(dir, "a.go"), "package main\n")
	mustRun(t, "git", "add", ".")
	mustRun(t, "git", "commit", "-m", "initial")
	mustWriteFile(t, filepath.Join(dir, "a.go"), "package main\n\nfunc main() {}\n")

	oldDiff, err := Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	res, err := DiffEx(dir)
	if err != nil {
		t.Fatalf("DiffEx: %v", err)
	}
	if oldDiff != res.Diff {
		t.Errorf("Diff() and DiffEx().Diff differ:\nDiff(): %q\nDiffEx(): %q", oldDiff, res.Diff)
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
		os.WriteFile("f.txt", []byte(content), 0600) //nolint:errcheck
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
	os.WriteFile("a.txt", []byte("x"), 0600) //nolint:errcheck
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
	os.WriteFile("a.txt", []byte("x"), 0600) //nolint:errcheck
	exec.Command("git", "add", "a.txt").Run() //nolint:errcheck
	exec.Command("git", "commit", "-m", "init").Run() //nolint:errcheck
	os.WriteFile("b.txt", []byte("y"), 0600) //nolint:errcheck
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
	os.WriteFile("top.txt", []byte("a"), 0600)           //nolint:errcheck
	os.WriteFile("sub/mid.txt", []byte("b"), 0600)       //nolint:errcheck
	os.WriteFile("sub/deep/bot.txt", []byte("c"), 0600)  //nolint:errcheck
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

func TestRepoRoot(t *testing.T) {
	tempDir, cleanup := newTempRepo(t)
	defer cleanup()

	root, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot failed: %v", err)
	}
	rootStat, err1 := os.Stat(root)
	tempStat, err2 := os.Stat(tempDir)
	if err1 != nil || err2 != nil || !os.SameFile(rootStat, tempStat) {
		t.Errorf("RepoRoot %q does not match tempDir %q", root, tempDir)
	}

	subDir := filepath.Join(tempDir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.Chdir(subDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}

	subRoot, err := RepoRoot()
	if err != nil {
		t.Fatalf("RepoRoot in subdir failed: %v", err)
	}
	subRootStat, err3 := os.Stat(subRoot)
	if err3 != nil || !os.SameFile(subRootStat, tempStat) {
		t.Errorf("sub RepoRoot %q does not match tempDir %q", subRoot, tempDir)
	}
}
