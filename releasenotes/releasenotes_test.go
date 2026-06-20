package releasenotes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildPrompt_includesEmoji(t *testing.T) {
	p := buildPrompt()
	if !strings.Contains(p, "🔧") {
		t.Fatal("prompt should reference emoji section headers")
	}
}

func TestBuildPrompt_endsWithCOMMITS(t *testing.T) {
	p := buildPrompt()
	if !strings.Contains(p, "COMMITS:") {
		t.Fatal("prompt should end with COMMITS:")
	}
}

func TestChat_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"## ✨ Features\n- New feature"}}]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	orig := openRouterURL
	openRouterURL = srv.URL
	defer func() { openRouterURL = orig }()

	got, err := chat(context.Background(), "model", "key", "system", "user")
	if err != nil {
		t.Fatal(err)
	}
	want := "## ✨ Features\n- New feature"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestChat_emptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"choices":[]}`)) //nolint:errcheck
	}))
	defer srv.Close()

	orig := openRouterURL
	openRouterURL = srv.URL
	defer func() { openRouterURL = orig }()

	_, err := chat(context.Background(), "model", "key", "system", "user")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestGetCommits_emptyRange(t *testing.T) {
	commits, err := getCommits(context.Background(), "HEAD", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected 0 commits, got %d", len(commits))
	}
}

func TestGenerate_noRefs(t *testing.T) {
	_, err := Generate(context.Background(), "nonexistent-tag-that-wont-exist", "HEAD", "key", "model", false)
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}

func TestBuildInitialPrompt(t *testing.T) {
	p := buildInitialPrompt()
	if !strings.Contains(p, "first release") {
		t.Fatal("initial prompt should mention first release")
	}
}
