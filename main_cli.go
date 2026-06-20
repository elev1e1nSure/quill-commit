package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// CLI holds parsed command-line flags and handles --help / --version / no-args.
type CLI struct {
	APIKey     string
	Model      string
	Interval   float64
	Stabilize  float64
	MaxDelays  int
	Preset     string
	Version    bool
	ShowUsage  bool
}

// Parse reads os.Args into CLI. It returns an error only for unknown flags.
// ShowUsage is set when the user requested help or ran with no arguments.
func (c *CLI) Parse() error {
	flag.CommandLine.Init(flag.CommandLine.Name(), flag.ContinueOnError)
	flag.CommandLine.SetOutput(&strings.Builder{})

	apiKeyFlag := flag.String("api-key", "", "OpenRouter API key (saved to credentials file for future runs)")
	modelFlag := flag.String("model", "", "model override (overrides quill.toml)")
	intervalFlag := flag.Float64("interval", 0, "check interval in minutes (overrides quill.toml)")
	stabilizeFlag := flag.Float64("stabilize", 0, "stabilization re-check interval in minutes (overrides quill.toml)")
	maxDelaysFlag := flag.Int("max-delays", 0, "max consecutive delays before forced commit (overrides quill.toml)")
	presetFlag := flag.String("preset", "", "config preset: active (default), deep, aggressive")
	versionFlag := flag.Bool("version", false, "print version and exit")

	if len(os.Args) == 1 {
		c.ShowUsage = true
		return nil
	}

	err := flag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			c.ShowUsage = true
			return nil
		}
		c.ShowUsage = true
		return fmt.Errorf("parse flags: %w", err)
	}

	c.APIKey = *apiKeyFlag
	c.Model = *modelFlag
	c.Interval = *intervalFlag
	c.Stabilize = *stabilizeFlag
	c.MaxDelays = *maxDelaysFlag
	c.Preset = *presetFlag
	c.Version = *versionFlag
	return nil
}
