package main

import (
	"fmt"
	"os"
	"path/filepath"

	"quill-commit/config"
	"quill-commit/git"
)

// ConfigResolver loads or creates quill.toml, applies preset and CLI overrides,
// and persists the config if anything changed.
type ConfigResolver struct {
	CLI CLI
}

// Resolve returns the final config and the repo root.
func (r *ConfigResolver) Resolve() (cfg config.Config, repoRoot string, err error) {
	repoRoot, err = git.RepoRoot()
	if err != nil {
		return config.Config{}, "", fmt.Errorf("get git repo root: %w", err)
	}

	configPath := filepath.Join(repoRoot, config.FileName)
	cfg, created, err := config.EnsureDefault(configPath)
	if err != nil {
		return config.Config{}, "", err
	}
	if created {
		fmt.Printf("created %s with defaults\n", configPath)
	}

	dirty := false
	individualDirty := false
	if r.CLI.Preset != "" {
		if !config.ApplyPreset(&cfg, r.CLI.Preset) {
			return config.Config{}, "", fmt.Errorf("unknown preset %q — valid presets: active, deep, aggressive", r.CLI.Preset)
		}
		dirty = true
	}
	if r.CLI.ModelSet {
		cfg.Model = r.CLI.Model
		dirty = true
		individualDirty = true
	}
	if r.CLI.IntervalSet {
		cfg.Interval = r.CLI.Interval
		dirty = true
		individualDirty = true
	}
	if r.CLI.StabilizeSet {
		cfg.Stabilize = r.CLI.Stabilize
		dirty = true
		individualDirty = true
	}
	if r.CLI.MaxDelaysSet {
		cfg.MaxDelays = r.CLI.MaxDelays
		dirty = true
		individualDirty = true
	}
	if r.CLI.StrategySet {
		if !config.ValidStrategy(r.CLI.Strategy) {
			return config.Config{}, "", fmt.Errorf("unknown strategy %q — valid values: permissive, standard, strict", r.CLI.Strategy)
		}
		cfg.Strategy = r.CLI.Strategy
		dirty = true
		individualDirty = true
	}
	if r.CLI.DelaysSet {
		enabled := r.CLI.DelaysAction == "enable"
		cfg.DelaysEnabled = &enabled
		dirty = true
		individualDirty = true
	}
	if individualDirty {
		cfg.Preset = "custom"
	}
	if dirty {
		if saveErr := config.Save(configPath, cfg); saveErr != nil {
			fmt.Fprintf(os.Stderr, "warn: could not save config: %v\n", saveErr)
		}
	}

	return cfg, repoRoot, nil
}
