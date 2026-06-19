package git

import (
	"fmt"
	"os/exec"
	"strings"
)

func Diff() (string, error) {
	out, err := exec.Command("git", "diff", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func Add() error {
	if err := exec.Command("git", "add", "-A").Run(); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	return nil
}

func Commit(message string) error {
	if err := exec.Command("git", "commit", "-m", message).Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func IsRepo() bool {
	return exec.Command("git", "rev-parse", "--git-dir").Run() == nil
}
