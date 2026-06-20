package config

import (
	"fmt"
	"math"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultInterval           = 2.0
	DefaultStabilize          = DefaultInterval / 2
	DefaultMaxDelays          = 0
	DefaultModel              = "deepseek/deepseek-v4-flash"
	DefaultIncludeContext     = true
	DefaultContextBudget      = 32000
	DefaultRecentCommitsCount = 10
	FileName                  = "quill.toml"
)

type Preset struct {
	Interval  float64
	Stabilize float64
	MaxDelays int
	Desc      string
}

var Presets = map[string]Preset{
	"active":     {Interval: 2, Stabilize: 1, MaxDelays: 3, Desc: "check every 2m, re-check every 1m — active coding sessions"},
	"deep":       {Interval: 5, Stabilize: 2.5, MaxDelays: 2, Desc: "check every 5m, re-check every 2.5m — long focused work"},
	"aggressive": {Interval: 0.5, Stabilize: 0.25, MaxDelays: 4, Desc: "check every 30s, re-check every 15s — frequent commits"},
}

func ApplyPreset(cfg *Config, name string) bool {
	p, ok := Presets[name]
	if !ok {
		return false
	}
	cfg.Interval = p.Interval
	cfg.Stabilize = p.Stabilize
	cfg.MaxDelays = p.MaxDelays
	return true
}

type Config struct {
	Interval           float64 `toml:"interval"`
	Stabilize          float64 `toml:"stabilize"`
	MaxDelays          int     `toml:"max_delays"`
	Model              string  `toml:"model"`
	IncludeContext     bool    `toml:"include_context"`
	ContextBudget      int     `toml:"context_budget"`
	RecentCommitsCount int     `toml:"recent_commits"`
	SessionID          string  `toml:"session_id"`
}

func Default() Config {
	return Config{
		Interval:           DefaultInterval,
		Stabilize:          DefaultStabilize,
		MaxDelays:          DefaultMaxDelays,
		Model:              DefaultModel,
		IncludeContext:     DefaultIncludeContext,
		ContextBudget:      DefaultContextBudget,
		RecentCommitsCount: DefaultRecentCommitsCount,
		SessionID:          "",
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

	if math.IsNaN(cfg.Interval) || math.IsInf(cfg.Interval, 0) || cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if math.IsNaN(cfg.Stabilize) || math.IsInf(cfg.Stabilize, 0) || cfg.Stabilize <= 0 {
		cfg.Stabilize = cfg.Interval / 2
	}
	if cfg.MaxDelays < 0 {
		cfg.MaxDelays = DefaultMaxDelays
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.ContextBudget <= 0 {
		cfg.ContextBudget = DefaultContextBudget
	}
	if cfg.RecentCommitsCount <= 0 {
		cfg.RecentCommitsCount = DefaultRecentCommitsCount
	}

	return cfg, nil
}

func Save(path string, cfg Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
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
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
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
