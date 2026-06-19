package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Interval != DefaultInterval {
		t.Errorf("expected Interval %d, got %d", DefaultInterval, cfg.Interval)
	}
	if cfg.MaxDelays != DefaultMaxDelays {
		t.Errorf("expected MaxDelays %d, got %d", DefaultMaxDelays, cfg.MaxDelays)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("expected Model %s, got %s", DefaultModel, cfg.Model)
	}
}

func TestLoadCreatesDefault(t *testing.T) {
	cfg, err := Load("nonexistent.toml")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got %v", err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("expected default interval, got %d", cfg.Interval)
	}
}

func TestLoadInvalidToml(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(tmp, []byte("invalid toml"), 0644); err != nil {
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
		Interval:  5,
		MaxDelays: 10,
		Model:     "test/model",
	}
	if err := Save(tmp, cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	loaded, err := Load(tmp)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.Interval != cfg.Interval {
		t.Errorf("interval mismatch: got %d", loaded.Interval)
	}
	if loaded.MaxDelays != cfg.MaxDelays {
		t.Errorf("maxDelays mismatch: got %d", loaded.MaxDelays)
	}
	if loaded.Model != cfg.Model {
		t.Errorf("model mismatch: got %s", loaded.Model)
	}
}

func TestLoadDefaultsInvalidValues(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "quill.toml")
	if err := os.WriteFile(tmp, []byte("interval = -1\nmax_delays = 0\nmodel = \"\""), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	cfg, err := Load(tmp)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("expected default interval, got %d", cfg.Interval)
	}
	if cfg.MaxDelays != DefaultMaxDelays {
		t.Errorf("expected default maxDelays, got %d", cfg.MaxDelays)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("expected default model, got %s", cfg.Model)
	}
}
