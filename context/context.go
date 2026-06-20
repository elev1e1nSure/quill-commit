package context

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"quill-commit/git"
)

type Static struct {
	Project     string
	Stack       string
	Packages    []string
	Conventions string
}

type Dynamic struct {
	RecentCommits string
	ChangedFiles  string
}

const Conventions = `Commit message rules:
- Must follow Conventional Commits format: type(scope): description
- Allowed types: feat, fix, refactor, perf, test, docs, chore, style, ci, build
- Scope is a short lowercase word, optional but preferred
- Description is imperative, lowercase, no period, max 72 characters
- Entire message must be under 100 characters
- Example: fix(ai): trim whitespace from model response`

var (
	lsFilesFunc       = git.LsFiles
	recentCommitsFunc = git.RecentCommits
	statusShortFunc   = git.StatusShort
)

// SetLsFilesFuncForTest overrides the git.LsFiles function for testing and returns a restore function.
func SetLsFilesFuncForTest(f func() (string, error)) func() {
	old := lsFilesFunc
	lsFilesFunc = f
	return func() { lsFilesFunc = old }
}

// SetRecentCommitsFuncForTest overrides the git.RecentCommits function for testing and returns a restore function.
func SetRecentCommitsFuncForTest(f func(int) (string, error)) func() {
	old := recentCommitsFunc
	recentCommitsFunc = f
	return func() { recentCommitsFunc = old }
}

// SetStatusShortFuncForTest overrides the git.StatusShort function for testing and returns a restore function.
func SetStatusShortFuncForTest(f func() (string, error)) func() {
	old := statusShortFunc
	statusShortFunc = f
	return func() { statusShortFunc = old }
}

func BuildStatic(repoRoot string) (Static, error) {
	var s Static
	s.Conventions = Conventions

	// 1. Packages from lsFilesFunc
	filesStr, err := lsFilesFunc()
	if err != nil {
		return s, fmt.Errorf("git ls-files: %w", err)
	}
	var pkgs []string
	seen := make(map[string]bool)
	if filesStr != "" {
		files := strings.Split(filesStr, "\n")
		for _, f := range files {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			parts := strings.SplitN(f, "/", 2)
			if len(parts) > 1 {
				pkg := parts[0]
				if strings.HasPrefix(pkg, ".") || pkg == "node_modules" {
					continue
				}
				if !seen[pkg] {
					seen[pkg] = true
					pkgs = append(pkgs, pkg)
				}
			}
		}
	}
	sort.Strings(pkgs)
	s.Packages = pkgs

	// 2. Project and Stack from CLAUDE.md > README.md > AGENTS.md
	docFiles := []string{"CLAUDE.md", "README.md", "AGENTS.md"}
	var docContent string
	var docFound bool
	for _, fn := range docFiles {
		p := filepath.Join(repoRoot, fn)
		data, err := os.ReadFile(p)
		if err == nil {
			docContent = string(data)
			docFound = true
			break
		}
	}

	if docFound {
		s.Project = parseSection(docContent, "Project")
		s.Stack = parseSection(docContent, "Stack")
	}

	return s, nil
}

func parseSection(content, sectionName string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	var sectionLines []string
	inSection := false
	targetHeader := "## " + sectionName

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if inSection {
				break
			}
			if strings.EqualFold(trimmed, targetHeader) {
				inSection = true
				continue
			}
		}
		if inSection {
			sectionLines = append(sectionLines, line)
		}
	}

	if !inSection {
		return ""
	}

	// Rejoin and trim space
	return strings.TrimSpace(strings.Join(sectionLines, "\n"))
}

func BuildDynamic(commitsN int) (Dynamic, error) {
	var d Dynamic
	var errs []error

	commits, err := recentCommitsFunc(commitsN)
	if err != nil {
		errs = append(errs, err)
	} else {
		d.RecentCommits = commits
	}

	status, err := statusShortFunc()
	if err != nil {
		errs = append(errs, err)
	} else {
		d.ChangedFiles = status
	}

	if len(errs) > 0 {
		return d, errors.Join(errs...)
	}
	return d, nil
}

func RenderSystem(s Static, budgetChars int) string {
	var projectStr string
	if s.Project != "" {
		projectStr = "## Project\n" + s.Project + "\n\n"
	}

	var stackStr string
	if s.Stack != "" {
		stackStr = "## Stack\n" + s.Stack + "\n\n"
	}

	var pkgStr string
	if len(s.Packages) > 0 {
		pkgStr = "## Packages\n"
		for _, p := range s.Packages {
			pkgStr += "- " + p + "\n"
		}
		pkgStr += "\n"
	}

	var convStr string
	if s.Conventions != "" {
		convStr = "## Conventions\n" + s.Conventions + "\n\n"
	}

	if budgetChars <= 0 {
		return projectStr + stackStr + pkgStr + convStr
	}

	// Full output
	full := projectStr + stackStr + pkgStr + convStr
	if len(full) <= budgetChars {
		return full
	}

	// Truncate order: drop Conventions first
	full = projectStr + stackStr + pkgStr
	if len(full) <= budgetChars {
		return full
	}

	// Drop Packages second
	full = projectStr + stackStr
	if len(full) <= budgetChars {
		return full
	}

	// Trim Stack, never Project
	if len(projectStr) >= budgetChars {
		return projectStr
	}

	available := budgetChars - len(projectStr)
	// Must fit "## Stack\n" and "\n\n" to show any content
	if available <= len("## Stack\n")+len("\n\n") {
		return projectStr
	}

	maxStackContentLen := available - len("## Stack\n") - len("\n\n")
	if len(s.Stack) <= maxStackContentLen {
		return projectStr + "## Stack\n" + s.Stack + "\n\n"
	}

	return projectStr + "## Stack\n" + safeSlice(s.Stack, maxStackContentLen) + "\n\n"
}

func safeSlice(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	sub := s[:maxBytes]
	for len(sub) > 0 {
		r, size := utf8.DecodeLastRuneInString(sub)
		if r != utf8.RuneError || size > 1 {
			break
		}
		sub = sub[:len(sub)-1]
	}
	return sub
}

func RenderUser(d Dynamic, diff string) string {
	var parts []string
	if d.RecentCommits != "" {
		parts = append(parts, "## Recent commits\n"+d.RecentCommits)
	}
	if d.ChangedFiles != "" {
		parts = append(parts, "## Changed files\n"+d.ChangedFiles)
	}
	if diff != "" {
		parts = append(parts, "## Diff\n"+diff)
	}
	return strings.Join(parts, "\n\n")
}

func (s Static) Hash() string {
	// sha256 hex of RenderSystem with a huge budget
	content := RenderSystem(s, 1<<20)
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}
