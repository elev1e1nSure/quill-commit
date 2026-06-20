package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Interval != DefaultInterval {
		t.Errorf("expected Interval %g, got %g", DefaultInterval, cfg.Interval)
	}
	if cfg.MaxDelays != DefaultMaxDelays {
		t.Errorf("expected MaxDelays %d, got %d", DefaultMaxDelays, cfg.MaxDelays)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("expected Model %s, got %s", DefaultModel, cfg.Model)
	}
	if cfg.IncludeContext != DefaultIncludeContext {
		t.Errorf("expected IncludeContext %t, got %t", DefaultIncludeContext, cfg.IncludeContext)
	}
	if cfg.ContextBudget != DefaultContextBudget {
		t.Errorf("expected ContextBudget %d, got %d", DefaultContextBudget, cfg.ContextBudget)
	}
	if cfg.RecentCommitsCount != DefaultRecentCommitsCount {
		t.Errorf("expected RecentCommitsCount %d, got %d", DefaultRecentCommitsCount, cfg.RecentCommitsCount)
	}
	if cfg.SessionID != "" {
		t.Errorf("expected SessionID empty, got %s", cfg.SessionID)
	}
}

func TestLoadCreatesDefault(t *testing.T) {
	cfg, err := Load("nonexistent.toml")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got %v", err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("expected default interval, got %g", cfg.Interval)
	}
}

func TestLoadInvalidToml(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(tmp, []byte("invalid toml"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	_, err := Load(tmp)
	if err == nil {
		t.Error("expected error for invalid toml")
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "quill.toml")
	cfg := Config{
		Interval:           5,
		MaxDelays:          10,
		Model:              "test/model",
		IncludeContext:     false,
		ContextBudget:      4000,
		RecentCommitsCount: 5,
		SessionID:          "test-session-123",
	}
	if err := Save(tmp, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := Load(tmp)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Interval != cfg.Interval {
		t.Errorf("interval mismatch: got %g", loaded.Interval)
	}
	if loaded.MaxDelays != cfg.MaxDelays {
		t.Errorf("maxDelays mismatch: got %d", loaded.MaxDelays)
	}
	if loaded.Model != cfg.Model {
		t.Errorf("model mismatch: got %s", loaded.Model)
	}
	if loaded.IncludeContext != cfg.IncludeContext {
		t.Errorf("includeContext mismatch: got %t", loaded.IncludeContext)
	}
	if loaded.ContextBudget != cfg.ContextBudget {
		t.Errorf("contextBudget mismatch: got %d", loaded.ContextBudget)
	}
	if loaded.RecentCommitsCount != cfg.RecentCommitsCount {
		t.Errorf("recentCommits mismatch: got %d", loaded.RecentCommitsCount)
	}
	if loaded.SessionID != cfg.SessionID {
		t.Errorf("sessionID mismatch: got %s", loaded.SessionID)
	}
}

func TestLoadDefaultsInvalidValues(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "quill.toml")
	if err := os.WriteFile(tmp, []byte("interval = -1\nmax_delays = -1\nmodel = \"\""), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("expected default interval, got %g", cfg.Interval)
	}
	if cfg.MaxDelays != DefaultMaxDelays {
		t.Errorf("expected default maxDelays, got %d", cfg.MaxDelays)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("expected default model, got %s", cfg.Model)
	}
}

func TestLoadContextDefaults(t *testing.T) {
	// Case 1: Load with include_context=false explicitly set, should stay false
	tmp1 := filepath.Join(t.TempDir(), "config1.toml")
	if err := os.WriteFile(tmp1, []byte("include_context = false\n"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	cfg1, err := Load(tmp1)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg1.IncludeContext {
		t.Error("expected IncludeContext to be false, got true")
	}
	// Missing context fields should get their defaults
	if cfg1.ContextBudget != DefaultContextBudget {
		t.Errorf("expected ContextBudget default %d, got %d", DefaultContextBudget, cfg1.ContextBudget)
	}
	if cfg1.RecentCommitsCount != DefaultRecentCommitsCount {
		t.Errorf("expected RecentCommitsCount default %d, got %d", DefaultRecentCommitsCount, cfg1.RecentCommitsCount)
	}
	if cfg1.SessionID != "" {
		t.Errorf("expected SessionID default empty, got %s", cfg1.SessionID)
	}

	// Case 2: Missing fields entirely should apply defaults
	tmp2 := filepath.Join(t.TempDir(), "config2.toml")
	if err := os.WriteFile(tmp2, []byte(""), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	cfg2, err := Load(tmp2)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg2.IncludeContext != DefaultIncludeContext {
		t.Errorf("expected IncludeContext default %t, got %t", DefaultIncludeContext, cfg2.IncludeContext)
	}
	if cfg2.ContextBudget != DefaultContextBudget {
		t.Errorf("expected ContextBudget default %d, got %d", DefaultContextBudget, cfg2.ContextBudget)
	}
	if cfg2.RecentCommitsCount != DefaultRecentCommitsCount {
		t.Errorf("expected RecentCommitsCount default %d, got %d", DefaultRecentCommitsCount, cfg2.RecentCommitsCount)
	}

	// Case 3: Invalid values (<= 0) should trigger zero-guards
	tmp3 := filepath.Join(t.TempDir(), "config3.toml")
	if err := os.WriteFile(tmp3, []byte("context_budget = 0\nrecent_commits = -1\n"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	cfg3, err := Load(tmp3)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg3.ContextBudget != DefaultContextBudget {
		t.Errorf("expected ContextBudget default %d for 0, got %d", DefaultContextBudget, cfg3.ContextBudget)
	}
	if cfg3.RecentCommitsCount != DefaultRecentCommitsCount {
		t.Errorf("expected RecentCommitsCount default %d for -1, got %d", DefaultRecentCommitsCount, cfg3.RecentCommitsCount)
	}
}
