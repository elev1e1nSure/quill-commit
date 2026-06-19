package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultInterval  = 2.0
	DefaultMaxDelays = 3
	DefaultModel     = "deepseek/deepseek-v4-flash"
	FileName         = "quill.toml"
)

type Preset struct {
	Interval  float64
	MaxDelays int
	Desc      string
}

var Presets = map[string]Preset{
	"active":     {Interval: 2, MaxDelays: 3, Desc: "check every 2m, stabilizes in 4m — good for active coding sessions"},
	"deep":       {Interval: 5, MaxDelays: 2, Desc: "check every 5m, stabilizes in 10m — for long focused work"},
	"aggressive": {Interval: 0.5, MaxDelays: 4, Desc: "check every 30s, stabilizes in 1m — frequent commits"},
}

func ApplyPreset(cfg *Config, name string) bool {
	p, ok := Presets[name]
	if !ok {
		return false
	}
	cfg.Interval = p.Interval
	cfg.MaxDelays = p.MaxDelays
	return true
}

type Config struct {
	Interval  float64 `toml:"interval"`
	MaxDelays int     `toml:"max_delays"`
	Model     string  `toml:"model"`
}

func Default() Config {
	return Config{
		Interval:  DefaultInterval,
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
	if err := Save(path, cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}
