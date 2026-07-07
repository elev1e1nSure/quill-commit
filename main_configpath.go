package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"quill-commit/credentials"
)

// configPathForRepo resolves the quill.toml location for a given repo root.
// Config files live per-repo under %APPDATA%\quill-commit\configs, keyed by
// a hash of the repo's absolute path — this keeps the repo directory itself
// free of quill-commit files while still giving each repo its own settings.
func configPathForRepo(repoRoot string) (string, error) {
	dir, err := credentials.Dir()
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repo path: %w", err)
	}

	key := strings.ToLower(filepath.ToSlash(abs))
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])[:12]

	fileName := fmt.Sprintf("%s-%s.toml", sanitizeRepoName(filepath.Base(abs)), hash)
	return filepath.Join(dir, "configs", fileName), nil
}

func sanitizeRepoName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "repo"
	}
	return b.String()
}
