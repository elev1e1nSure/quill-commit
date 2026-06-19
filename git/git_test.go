package git

import (
	"testing"
)

func TestIsRepoTrue(t *testing.T) {
	if !IsRepo() {
		t.Error("expected true in git repo")
	}
}

func TestDiffEmpty(t *testing.T) {
	diff, err := Diff()
	if err != nil {
		t.Fatalf("Diff() failed: %v", err)
	}
	if diff == "" {
		t.Log("diff is empty (clean working tree)")
	}
}

func TestAddNoError(t *testing.T) {
	if err := Add(); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}
}
