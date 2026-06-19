package context

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildStatic(t *testing.T) {
	// Mock lsFilesFunc
	oldLs := lsFilesFunc
	defer func() { lsFilesFunc = oldLs }()

	lsFilesFunc = func() (string, error) {
		return "ai/ai.go\nconfig/config.go\ngit/git.go\nmain.go\n", nil
	}

	tmpDir := t.TempDir()

	// Write mock CLAUDE.md
	claudeContent := `## Project
This is the project description.
It spans multiple lines.

## Stack
- Go 1.24
- TOML

## Other Section
Some other content.`

	if err := os.WriteFile(filepath.Join(tmpDir, "CLAUDE.md"), []byte(claudeContent), 0644); err != nil {
		t.Fatalf("failed to write CLAUDE.md: %v", err)
	}

	s, err := BuildStatic(tmpDir)
	if err != nil {
		t.Fatalf("BuildStatic failed: %v", err)
	}

	if s.Project != "This is the project description.\nIt spans multiple lines." {
		t.Errorf("unexpected Project: %q", s.Project)
	}
	if s.Stack != "- Go 1.24\n- TOML" {
		t.Errorf("unexpected Stack: %q", s.Stack)
	}
	if len(s.Packages) != 3 || s.Packages[0] != "ai" || s.Packages[1] != "config" || s.Packages[2] != "git" {
		t.Errorf("unexpected Packages: %v", s.Packages)
	}
	if s.Conventions != Conventions {
		t.Errorf("unexpected Conventions")
	}
}

func TestBuildStaticFallbackAndMissing(t *testing.T) {
	oldLs := lsFilesFunc
	defer func() { lsFilesFunc = oldLs }()
	lsFilesFunc = func() (string, error) { return "", nil }

	tmpDir := t.TempDir()

	// Write mock README.md since CLAUDE.md is missing
	readmeContent := `## Project
README Project.

## Stack
README Stack.`

	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(readmeContent), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}

	s, err := BuildStatic(tmpDir)
	if err != nil {
		t.Fatalf("BuildStatic failed: %v", err)
	}
	if s.Project != "README Project." {
		t.Errorf("expected README Project, got %q", s.Project)
	}

	// No doc file at all
	emptyDir := t.TempDir()
	s2, err := BuildStatic(emptyDir)
	if err != nil {
		t.Fatalf("BuildStatic empty failed: %v", err)
	}
	if s2.Project != "" || s2.Stack != "" {
		t.Errorf("expected empty Project/Stack, got %+v", s2)
	}
}

func TestBuildDynamic(t *testing.T) {
	oldRecent := recentCommitsFunc
	oldStatus := statusShortFunc
	defer func() {
		recentCommitsFunc = oldRecent
		statusShortFunc = oldStatus
	}()

	// Happy path
	recentCommitsFunc = func(n int) (string, error) {
		return "commit1\ncommit2", nil
	}
	statusShortFunc = func() (string, error) {
		return "M ai/ai.go", nil
	}

	d, err := BuildDynamic(2)
	if err != nil {
		t.Fatalf("BuildDynamic failed: %v", err)
	}
	if d.RecentCommits != "commit1\ncommit2" {
		t.Errorf("unexpected RecentCommits: %q", d.RecentCommits)
	}
	if d.ChangedFiles != "M ai/ai.go" {
		t.Errorf("unexpected ChangedFiles: %q", d.ChangedFiles)
	}

	// Failure path (partial results + errors)
	recentCommitsFunc = func(n int) (string, error) {
		return "", errors.New("recent commits error")
	}
	statusShortFunc = func() (string, error) {
		return "M config/config.go", nil
	}

	d2, err := BuildDynamic(2)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if d2.ChangedFiles != "M config/config.go" {
		t.Errorf("expected partial ChangedFiles, got %q", d2.ChangedFiles)
	}
	if d2.RecentCommits != "" {
		t.Errorf("expected empty RecentCommits, got %q", d2.RecentCommits)
	}
}

func TestRenderSystemBudgetAndTruncation(t *testing.T) {
	s := Static{
		Project:     "Project A",
		Stack:       "Go, SQLite",
		Packages:    []string{"ai", "git"},
		Conventions: "Rule 1\nRule 2",
	}

	// Case 1: budget <= 0 (unlimited)
	outUnlimited := RenderSystem(s, 0)
	expectedFull := "## Project\nProject A\n\n## Stack\nGo, SQLite\n\n## Packages\n- ai\n- git\n\n## Conventions\nRule 1\nRule 2\n\n"
	if outUnlimited != expectedFull {
		t.Errorf("expected full output, got %q", outUnlimited)
	}

	// Case 2: budget fits full
	if RenderSystem(s, len(expectedFull)+10) != expectedFull {
		t.Error("expected full output for large budget")
	}

	// Case 3: budget truncated - drop Conventions
	outTrunc1 := RenderSystem(s, 80)
	expectedTrunc1 := "## Project\nProject A\n\n## Stack\nGo, SQLite\n\n## Packages\n- ai\n- git\n\n"
	if outTrunc1 != expectedTrunc1 {
		t.Errorf("expected first truncation, got %q", outTrunc1)
	}

	// Case 4: budget truncated - drop Packages
	outTrunc2 := RenderSystem(s, 50)
	expectedTrunc2 := "## Project\nProject A\n\n## Stack\nGo, SQLite\n\n"
	if outTrunc2 != expectedTrunc2 {
		t.Errorf("expected second truncation, got %q", outTrunc2)
	}

	// Case 5: budget truncated - trim Stack
	outTrunc3 := RenderSystem(s, 30)
	if outTrunc3 != "## Project\nProject A\n\n" {
		t.Errorf("expected Stack dropped completely, got %q", outTrunc3)
	}

	outTrunc4 := RenderSystem(s, 38)
	expectedTrunc4 := "## Project\nProject A\n\n## Stack\nGo, S\n\n"
	if outTrunc4 != expectedTrunc4 {
		t.Errorf("expected Stack content trimmed to 5 chars, got %q", outTrunc4)
	}
}

func TestRenderSystemBudgetOneNoPanic(t *testing.T) {
	s := Static{
		Project:     "Project A",
		Stack:       "Go, SQLite",
		Packages:    []string{"ai", "git"},
		Conventions: "Rule 1",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RenderSystem(s, 1) panicked: %v", r)
		}
	}()

	out := RenderSystem(s, 1)
	expected := "## Project\nProject A\n\n"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}
}

func TestRenderUser(t *testing.T) {
	d := Dynamic{
		RecentCommits: "commit1",
		ChangedFiles:  "M f.txt",
	}
	out := RenderUser(d, "diff content")
	expected := "## Recent commits\ncommit1\n\n## Changed files\nM f.txt\n\n## Diff\ndiff content"
	if out != expected {
		t.Errorf("expected %q, got %q", expected, out)
	}

	d2 := Dynamic{
		RecentCommits: "",
		ChangedFiles:  "M f.txt",
	}
	out2 := RenderUser(d2, "")
	expected2 := "## Changed files\nM f.txt"
	if out2 != expected2 {
		t.Errorf("expected %q, got %q", expected2, out2)
	}
}

func TestHash(t *testing.T) {
	s1 := Static{
		Project:  "P1",
		Stack:    "S1",
		Packages: []string{"ai", "git"},
	}
	s2 := Static{
		Project:  "P1",
		Stack:    "S1",
		Packages: []string{"git", "ai"},
	}

	h1a := s1.Hash()
	h1b := s1.Hash()
	if h1a != h1b {
		t.Error("Hash twice called on same Static should be identical")
	}

	h2 := s2.Hash()
	if h1a == h2 {
		t.Error("Hash should be different if Packages order changes")
	}
}
