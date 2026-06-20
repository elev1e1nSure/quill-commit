package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"quill-commit/pathfilter"
	"quill-commit/secretscan"
)

func runGit(args ...string) (string, error) {
	return runGitWithStdin("", args...)
}

func runGitWithStdin(stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return s, nil
}

func readHeader(path string, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

func quotePath(p string) string {
	if strings.ContainsAny(p, " \t\"\\") {
		return `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return p
}

func isKnownBinaryExtension(path string) bool {
	exts := []string{
		".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp", ".pdf",
		".zip", ".gz", ".tar", ".tgz", ".bz2", ".xz", ".7z",
		".exe", ".dll", ".so", ".dylib", ".a", ".o", ".pyc",
		".db", ".sqlite", ".class", ".jar", ".war", ".ear",
		".woff", ".woff2", ".eot", ".ttf", ".mp3", ".mp4", ".wav",
	}
	p := strings.ToLower(path)
	for _, ext := range exts {
		if strings.HasSuffix(p, ext) {
			return true
		}
	}
	return false
}

// DiffResult holds the filtered diff along with included and excluded files.
type DiffResult struct {
	Diff          string
	IncludedFiles []string
	ExcludedFiles []string
}

// Diff returns the raw diff string (backward compatible).
// It delegates to DiffEx("") which applies hardcoded path and content
// filters, but does not load a user-defined .quillignore (no repoRoot).
func Diff() (string, error) {
	res, err := DiffEx("")
	return res.Diff, err
}

// DiffEx returns a diff with three layers of filtering:
//  1. Path filter (hardcoded secrets + .quillignore).
//  2. Content scan (known secret signatures in untracked files and added lines).
//  3. It also returns the list of included files so callers can stage only them.
func DiffEx(repoRoot string) (DiffResult, error) {
	var result DiffResult

	filter := pathfilter.New()
	if repoRoot != "" {
		ignorePath := filepath.Join(repoRoot, ".quillignore")
		if err := filter.LoadIgnoreFile(ignorePath); err != nil && !os.IsNotExist(err) {
			// Non-fatal: log would go here if we had a logger in this package.
			// We silently proceed with hardcoded patterns only.
		}
	}

	// --- Tracked files ---
	trackedNames, err := runGit("diff", "HEAD", "--name-only")
	if err != nil {
		return result, err
	}

	trackedIncluded := []string{}
	for _, name := range strings.Fields(trackedNames) {
		if name == "log.txt" || strings.HasSuffix(name, "/log.txt") || strings.HasSuffix(name, "\\log.txt") {
			result.ExcludedFiles = append(result.ExcludedFiles, name)
			continue
		}
		if filter.IsExcluded(name) {
			result.ExcludedFiles = append(result.ExcludedFiles, name)
			continue
		}
		trackedIncluded = append(trackedIncluded, name)
	}

	trackedDiff := ""
	if len(trackedIncluded) > 0 {
		args := append([]string{"diff", "HEAD", "--"}, trackedIncluded...)
		args = append(args, ":(exclude)log.txt")
		trackedDiff, err = runGit(args...)
		if err != nil {
			return result, err
		}
	}

	// Content scan on tracked diff: remove any file whose added lines contain a secret.
	if trackedDiff != "" {
		trackedDiff, trackedIncluded = filterTrackedDiff(trackedDiff, trackedIncluded, &result.ExcludedFiles)
	}

	// --- Untracked files ---
	untracked, err := runGit("ls-files", "--others", "--exclude-standard")
	if err != nil {
		// If ls-files fails, we still have the tracked diff.
		result.Diff = trackedDiff
		result.IncludedFiles = trackedIncluded
		return result, nil
	}

	untrackedFiles := strings.Fields(untracked)
	var untrackedBuilder strings.Builder
	for _, f := range untrackedFiles {
		if f == "log.txt" || strings.HasSuffix(f, "/log.txt") || strings.HasSuffix(f, "\\log.txt") {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		if filter.IsExcluded(f) {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		if isKnownBinaryExtension(f) {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		info, err := os.Stat(f)
		if err != nil || info.Size() > 1024*1024 { // 1MB limit
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		header, err := readHeader(f, 8192)
		if err != nil {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		if isBinary(header) {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		if secretscan.Scan(header) {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		content, err := os.ReadFile(f)
		if err != nil {
			result.ExcludedFiles = append(result.ExcludedFiles, f)
			continue
		}
		lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
		qPath := quotePath(f)
		fmt.Fprintf(&untrackedBuilder, "diff --git a/%s b/%s\n", qPath, qPath)
		fmt.Fprintf(&untrackedBuilder, "new file mode 100644\n")
		fmt.Fprintf(&untrackedBuilder, "--- /dev/null\n+++ b/%s\n", qPath)
		fmt.Fprintf(&untrackedBuilder, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, l := range lines {
			fmt.Fprintf(&untrackedBuilder, "+%s\n", l)
		}
		result.IncludedFiles = append(result.IncludedFiles, f)
	}

	result.Diff = trackedDiff
	if untrackedBuilder.Len() > 0 {
		if result.Diff != "" {
			result.Diff += "\n"
		}
		result.Diff += untrackedBuilder.String()
	}
	result.IncludedFiles = append(trackedIncluded, result.IncludedFiles...)

	return result, nil
}

// filterTrackedDiff scans the raw diff for secret signatures in added lines.
// If a file contains a secret in an added line, its entire diff section is
// removed, the file is moved from included to excluded, and the cleaned diff
// is returned.
func filterTrackedDiff(rawDiff string, included []string, excluded *[]string) (string, []string) {
	lines := strings.Split(rawDiff, "\n")
	var output strings.Builder
	var currentSection strings.Builder
	var currentFile string
	var currentHasSecret bool
	var inSection bool

	includedSet := make(map[string]struct{}, len(included))
	for _, f := range included {
		includedSet[f] = struct{}{}
	}

	flushSection := func() {
		if !inSection {
			return
		}
		if currentHasSecret {
			*excluded = append(*excluded, currentFile)
			delete(includedSet, currentFile)
		} else if currentSection.Len() > 0 {
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(currentSection.String())
		}
		currentSection.Reset()
		currentFile = ""
		currentHasSecret = false
		inSection = false
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git a/") {
			flushSection()
			currentFile = parseDiffFile(line)
			inSection = true
			currentSection.WriteString(line)
			currentSection.WriteString("\n")
			continue
		}
		if !inSection {
			// Header lines before the first diff --git (shouldn't happen in normal git diff).
			if output.Len() > 0 {
				output.WriteString("\n")
			}
			output.WriteString(line)
			output.WriteString("\n")
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			if secretscan.Scan([]byte(line)) {
				currentHasSecret = true
			}
		}
		currentSection.WriteString(line)
		currentSection.WriteString("\n")
	}
	flushSection()

	newIncluded := make([]string, 0, len(includedSet))
	for _, f := range included {
		if _, ok := includedSet[f]; ok {
			newIncluded = append(newIncluded, f)
		}
	}
	return strings.TrimSuffix(output.String(), "\n"), newIncluded
}

// parseDiffFile extracts the b/ path from a "diff --git a/... b/..." line.
func parseDiffFile(line string) string {
	// Format: diff --git a/<path> b/<path>
	parts := strings.SplitN(line, " b/", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func RecentCommits(n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	args := fmt.Sprintf("-%d", n)
	out, err := runGit("log", "--oneline", args)
	if err != nil {
		if strings.Contains(out, "does not have any commits") {
			return "", nil
		}
		return "", err
	}
	return out, nil
}

func StatusShort() (string, error) {
	out, err := runGit("status", "--short")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", nil
	}
	return out, nil
}

func LsFiles() (string, error) {
	out, err := runGit("ls-files")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "", nil
	}
	return out, nil
}

// isBinary checks if data looks binary (null byte in first 8k).
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func Add() error {
	_, err := runGit("add", "-A")
	return err
}

func AddPaths(paths []string) error {
	args := append([]string{"add", "--"}, paths...)
	_, err := runGit(args...)
	return err
}

func Commit(message string) error {
	out, err := runGitWithStdin(message, "commit", "-F", "-")
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func HeadMessage() (string, error) {
	return runGit("log", "-1", "--format=%B")
}

func AmendCommit(message string) error {
	out, err := runGitWithStdin(message, "commit", "--amend", "-F", "-")
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func IsRepo() bool {
	_, err := runGit("rev-parse", "--git-dir")
	return err == nil
}

func HeadHash() string {
	out, err := runGit("rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

func RepoRoot() (string, error) {
	return runGit("rev-parse", "--show-toplevel")
}
