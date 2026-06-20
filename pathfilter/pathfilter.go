package pathfilter

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Filter holds path exclusion patterns.
type Filter struct {
	secrets []string // hardcoded secret patterns
	user    []string // from .quillignore
}

// defaultSecretPatterns are hardcoded paths that should never be staged or sent to AI.
var defaultSecretPatterns = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"*_rsa",
	"*.p12",
	"credentials*",
	"secrets*",
}

// New creates a filter with the built-in secret patterns.
func New() *Filter {
	return &Filter{
		secrets: append([]string{}, defaultSecretPatterns...),
	}
}

// LoadIgnoreFile reads a .quillignore file (same syntax as .gitignore) and
// appends its patterns to the user-defined exclusions.
func (f *Filter) LoadIgnoreFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f.user = append(f.user, line)
	}
	return scanner.Err()
}

// IsExcluded reports whether a path matches any secret or user-defined pattern.
func (f *Filter) IsExcluded(path string) bool {
	// Normalize to forward slashes for matching.
	path = filepath.ToSlash(path)
	basename := filepath.Base(path)

	for _, p := range f.secrets {
		if matchPattern(p, path, basename) {
			return true
		}
	}
	for _, p := range f.user {
		if matchPattern(p, path, basename) {
			return true
		}
	}
	return false
}

// matchPattern tries doublestar.Match against the full path and the basename.
// For directory patterns (ending with "/") it also matches anything inside that directory.
// For literal names without wildcards it also checks exact basename equality.
func matchPattern(pattern, path, basename string) bool {
	isDirPattern := strings.HasSuffix(pattern, "/")
	// Strip trailing slash for matching, but remember it was a directory pattern.
	pattern = strings.TrimSuffix(pattern, "/")

	matched, err := doublestar.Match(pattern, path)
	if err == nil && matched {
		return true
	}
	matched, err = doublestar.Match(pattern, basename)
	if err == nil && matched {
		return true
	}

	// Directory pattern: match contents inside the directory.
	if isDirPattern {
		if path == pattern || strings.HasPrefix(path, pattern+"/") {
			return true
		}
		// Also try doublestar glob for recursive match.
		matched, err = doublestar.Match(pattern+"/**", path)
		if err == nil && matched {
			return true
		}
	}

	// Also allow exact basename match for literal names (e.g. "credentials").
	if !strings.ContainsAny(pattern, "*?[") {
		if basename == pattern {
			return true
		}
	}
	return false
}
