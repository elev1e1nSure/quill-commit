package pathfilter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecrets(t *testing.T) {
	f := New()

	cases := []struct {
		path     string
		excluded bool
	}{
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{"key.pem", true},
		{"id_rsa", true},
		{"deploy_rsa", true},
		{"cert.p12", true},
		{"credentials", true},
		{"credentials.json", true},
		{"secrets.yaml", true},
		{"secrets", true},
		{"main.go", false},
		{"readme.md", false},
		{"foo.pem.txt", false}, // not a .pem file
		{".env/dir", false},    // directory named .env
	}

	for _, tc := range cases {
		if got := f.IsExcluded(tc.path); got != tc.excluded {
			t.Errorf("IsExcluded(%q) = %v, want %v", tc.path, got, tc.excluded)
		}
	}
}

func TestUserPatterns(t *testing.T) {
	f := New()
	// Simulate .quillignore content.
	tmpDir := t.TempDir()
	ignorePath := filepath.Join(tmpDir, ".quillignore")
	if err := os.WriteFile(ignorePath, []byte("*.log\ntemp/\nbuild/**\n"), 0644); err != nil {
		t.Fatalf("write .quillignore: %v", err)
	}

	if err := f.LoadIgnoreFile(ignorePath); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path     string
		excluded bool
	}{
		{"debug.log", true},
		{"temp/file.go", true},
		{"build/output.js", true},
		{"build/nested/deep/file.css", true},
		{"main.go", false},
	}

	for _, tc := range cases {
		if got := f.IsExcluded(tc.path); got != tc.excluded {
			t.Errorf("IsExcluded(%q) = %v, want %v", tc.path, got, tc.excluded)
		}
	}
}

func TestQuillIgnoreParsing(t *testing.T) {
	f := New()
	tmpDir := t.TempDir()
	ignorePath := filepath.Join(tmpDir, ".quillignore")
	content := `
# This is a comment

*.log

   temp/   

# Another comment
build
`
	if err := os.WriteFile(ignorePath, []byte(content), 0644); err != nil {
		t.Fatalf("write .quillignore: %v", err)
	}

	if err := f.LoadIgnoreFile(ignorePath); err != nil {
		t.Fatal(err)
	}

	if !f.IsExcluded("app.log") {
		t.Error("expected app.log to be excluded")
	}
	if !f.IsExcluded("temp/") {
		t.Error("expected temp/ to be excluded")
	}
	if !f.IsExcluded("build") {
		t.Error("expected build to be excluded")
	}
	if f.IsExcluded("main.go") {
		t.Error("expected main.go to not be excluded")
	}
}

func TestEdgeCases(t *testing.T) {
	f := New()

	cases := []struct {
		path     string
		excluded bool
	}{
		{"dir/.env", true},
		{"dir/key.pem", true},
		{"dir/credentials", true},
		{"dir/secrets.json", true},
		{"foo.env.bar", false}, // not .env.* because dot is not at start
	}

	for _, tc := range cases {
		if got := f.IsExcluded(tc.path); got != tc.excluded {
			t.Errorf("IsExcluded(%q) = %v, want %v", tc.path, got, tc.excluded)
		}
	}
}
