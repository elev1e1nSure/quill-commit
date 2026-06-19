package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Diff() (string, error) {
	out, err := exec.Command("git", "diff", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	s := strings.TrimSpace(string(out))

	untracked, err := exec.Command("git", "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return s, nil
	}
	files := strings.Fields(string(untracked))
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
	out, err := exec.Command("git", "add", "-A").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Commit(message string) error {
	cmd := exec.Command("git", "commit", "-F", "-")
	cmd.Stdin = strings.NewReader(message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func IsRepo() bool {
	return exec.Command("git", "rev-parse", "--git-dir").Run() == nil
}

func HeadHash() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
