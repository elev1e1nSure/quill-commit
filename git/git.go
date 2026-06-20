package git

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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

func Diff() (string, error) {
	s, err := runGit("diff", "HEAD", "--", ".", ":(exclude)log.txt")
	if err != nil {
		return "", err
	}

	untracked, err := runGit("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return s, nil
	}
	files := strings.Fields(untracked)
	for _, f := range files {
		if f == "log.txt" || strings.HasSuffix(f, "/log.txt") || strings.HasSuffix(f, "\\log.txt") {
			continue
		}
		if isKnownBinaryExtension(f) {
			continue
		}
		info, err := os.Stat(f)
		if err != nil || info.Size() > 1024*1024 { // 1MB limit to avoid OOM
			continue
		}
		header, err := readHeader(f, 8192)
		if err != nil {
			continue
		}
		if isBinary(header) {
			continue
		}
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
		var b strings.Builder
		qPath := quotePath(f)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n", qPath, qPath)
		fmt.Fprintf(&b, "new file mode 100644\n")
		fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n", qPath)
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
