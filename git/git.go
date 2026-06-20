package git

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
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

func Diff() (string, error) {
	s, err := runGit("diff", "HEAD")
	if err != nil {
		return "", err
	}

	untracked, err := runGit("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return s, nil
	}
	files := strings.Fields(untracked)
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if isBinary(content) {
			continue
		}
		lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
		var b strings.Builder
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", f, f)
		fmt.Fprintf(&b, "new file mode 100644\n")
		fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", f)
		fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, l := range lines {
			fmt.Fprintf(&b, "+%s\n", l)
		}
		if s != "" {
			s += "\n"
		}
		s += b.String()
	}
	return s, nil
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
	files := strings.Split(out, "\n")
	sort.Strings(files)
	return strings.Join(files, "\n"), nil
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

func Commit(message string) error {
	_, err := runGitWithStdin(message, "commit", "-F", "-")
	return err
}

func HeadMessage() (string, error) {
	return runGit("log", "-1", "--format=%B")
}

func AmendCommit(message string) error {
	_, err := runGitWithStdin(message, "commit", "--amend", "-F", "-")
	return err
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
