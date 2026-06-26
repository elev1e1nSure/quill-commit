package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultInterval           = 120
	DefaultStabilize          = 60
	DefaultMaxDelays          = 0
	DefaultModel              = "deepseek/deepseek-v4-flash"
	DefaultIncludeContext     = true
	DefaultContextBudget      = 32000
	DefaultRecentCommitsCount = 10
	DefaultStrategy           = "standard"
	DefaultDelaysEnabled      = false
	FileName                  = "quill.toml"
)

type Preset struct {
	Interval  int
	Stabilize int
	MaxDelays int
	Desc      string
}

var presets = map[string]Preset{
	"active":     {Interval: 120, Stabilize: 60, MaxDelays: 3, Desc: "check every 2m, re-check every 1m — active coding sessions"},
	"deep":       {Interval: 300, Stabilize: 150, MaxDelays: 2, Desc: "check every 5m, re-check every 2.5m — long focused work"},
	"aggressive": {Interval: 30, Stabilize: 15, MaxDelays: 4, Desc: "check every 30s, re-check every 15s — frequent commits"},
}

func ApplyPreset(cfg *Config, name string) bool {
	p, ok := presets[name]
	if !ok {
		return false
	}
	cfg.Interval = p.Interval
	cfg.Stabilize = p.Stabilize
	cfg.MaxDelays = p.MaxDelays
	cfg.Preset = name
	return true
}

type Config struct {
	Interval           int    `toml:"interval"`
	Stabilize          int    `toml:"stabilize"`
	MaxDelays          int    `toml:"max_delays"`
	Model              string `toml:"model"`
	Strategy           string `toml:"strategy"`
	Preset             string `toml:"preset"`
	IncludeContext     bool   `toml:"include_context"`
	ContextBudget      int    `toml:"context_budget"`
	RecentCommitsCount int    `toml:"recent_commits"`
	SessionID          string `toml:"session_id"`
	DelaysEnabled      *bool  `toml:"delays_enabled"`
}

// ValidStrategy reports whether s is a known commit strategy name.
func ValidStrategy(s string) bool {
	return s == "permissive" || s == "standard" || s == "strict"
}

// Delayed reports whether delay requests from the model are enabled.
// Returns false when DelaysEnabled is nil (default) or explicitly false.
func (c Config) Delayed() bool {
	return c.DelaysEnabled != nil && *c.DelaysEnabled
}

func Default() Config {
	delaysEnabled := false
	return Config{
		Interval:           DefaultInterval,
		Stabilize:          DefaultStabilize,
		MaxDelays:          DefaultMaxDelays,
		Model:              DefaultModel,
		Strategy:           DefaultStrategy,
		IncludeContext:     DefaultIncludeContext,
		ContextBudget:      DefaultContextBudget,
		RecentCommitsCount: DefaultRecentCommitsCount,
		SessionID:          "",
		DelaysEnabled:      &delaysEnabled,
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.Stabilize <= 0 {
		cfg.Stabilize = DefaultStabilize
	}
	if cfg.MaxDelays < 0 {
		cfg.MaxDelays = DefaultMaxDelays
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if !ValidStrategy(cfg.Strategy) {
		cfg.Strategy = DefaultStrategy
	}
	if cfg.ContextBudget <= 0 {
		cfg.ContextBudget = DefaultContextBudget
	}
	if cfg.RecentCommitsCount <= 0 {
		cfg.RecentCommitsCount = DefaultRecentCommitsCount
	}
	if cfg.DelaysEnabled == nil {
		delaysEnabled := true
		cfg.DelaysEnabled = &delaysEnabled
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func EnsureDefault(path string) (Config, bool, error) {
	_, statErr := os.Stat(path)
	if statErr == nil {
		cfg, err := Load(path)
		if err != nil {
			return Config{}, false, fmt.Errorf("load config: %w", err)
		}
		return cfg, false, nil
	}
	if !os.IsNotExist(statErr) {
		return Config{}, false, fmt.Errorf("stat config: %w", statErr)
	}

	cfg := Default()
	data, err := toml.Marshal(cfg)
	if err != nil {
		return Config{}, false, fmt.Errorf("marshal config: %w", err)
	}

	// Atomic create — O_EXCL fails if another process already created the file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Another process won the race — reload their config
			cfg, err := Load(path)
			if err != nil {
				return Config{}, false, fmt.Errorf("reload after race: %w", err)
			}
			return cfg, true, nil
		}
		return Config{}, false, fmt.Errorf("create config: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		return Config{}, false, fmt.Errorf("write config: %w", err)
	}

	if err := f.Close(); err != nil {
		return Config{}, false, fmt.Errorf("close config: %w", err)
	}

	return cfg, true, nil
}
