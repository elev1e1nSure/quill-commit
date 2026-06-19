package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultInterval  = 2.0
	DefaultStabilize = DefaultInterval / 2
	DefaultMaxDelays = 3
	DefaultModel     = "deepseek/deepseek-v4-flash"
	FileName         = "quill.toml"
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
	Interval  float64 `toml:"interval"`
	Stabilize float64 `toml:"stabilize"`
	MaxDelays int     `toml:"max_delays"`
	Model     string  `toml:"model"`
}

func Default() Config {
	return Config{
		Interval:  DefaultInterval,
		Stabilize: DefaultStabilize,
		MaxDelays: DefaultMaxDelays,
		Model:     DefaultModel,
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

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Interval <= 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.Stabilize <= 0 {
		cfg.Stabilize = cfg.Interval / 2
	}
	if cfg.MaxDelays <= 0 {
		cfg.MaxDelays = DefaultMaxDelays
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
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
	cfg, err := Load(path)
	if err == nil {
		return cfg, false, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, false, fmt.Errorf("load config: %w", err)
	}

	cfg = Default()
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
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return Config{}, false, fmt.Errorf("write config: %w", err)
	}

	return cfg, true, nil
}
