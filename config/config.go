package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

const (
	DefaultInterval  = 10
	DefaultMaxDelays = 3
	DefaultModel     = "deepseek/deepseek-v4-flash"
	FileName         = "quill.toml"
)

type Config struct {
	Interval  int    `toml:"interval"`
	MaxDelays int    `toml:"max_delays"`
	Model     string `toml:"model"`
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
	info, statErr := os.Stat(path)
	if statErr == nil && info != nil {
		cfg, err := Load(path)
		return cfg, false, err
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return Config{}, false, fmt.Errorf("stat config: %w", statErr)
	}
	cfg := Default()
	if err := Save(path, cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}
