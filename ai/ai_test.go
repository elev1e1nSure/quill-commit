package ai

import (
	"testing"
)

func TestFallback(t *testing.T) {
	d := fallback()
	if !d.Commit {
		t.Error("fallback should set Commit=true")
	}
	if d.Message != "auto: fallback commit" {
		t.Errorf("fallback message mismatch: %s", d.Message)
	}
}

func TestAskEmptyDiff(t *testing.T) {
	d, err := Ask("", "model", "key")
	if err != nil {
		t.Fatalf("Ask with empty diff failed: %v", err)
	}
	if d.Commit {
		t.Error("empty diff should not commit")
	}
	if d.Delay != 1 {
		t.Errorf("expected delay 1, got %d", d.Delay)
	}
}
