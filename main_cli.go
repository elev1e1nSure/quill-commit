package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// CLI holds parsed command-line flags and handles --help / --version / no-args.
type CLI struct {
	APIKey        string
	Model         string
	Interval      int
	Stabilize     int
	MaxDelays     int
	Preset        string
	Strategy      string
	Version       bool
	ShowUsage     bool
	PathAction    string
	DelaysAction  string
	IntervalSet   bool
	StabilizeSet  bool
	MaxDelaysSet  bool
	ModelSet      bool
	StrategySet   bool
	PresetSet     bool
	DelaysSet     bool
}

// Parse reads os.Args into CLI. It returns an error only for unknown flags.
// ShowUsage is set when the user requested help or ran with no arguments.
func (c *CLI) Parse() error {
	flag.CommandLine.Init(flag.CommandLine.Name(), flag.ContinueOnError)
	flag.CommandLine.SetOutput(&strings.Builder{})

	apiKeyFlag := flag.String("api-key", "", "OpenRouter API key (saved to credentials file for future runs)")
	modelFlag := flag.String("model", "", "model to use (saved to quill.toml)")
	intervalFlag := flag.Int("interval", 0, "check interval in seconds (saved to quill.toml)")
	stabilizeFlag := flag.Int("stabilize", 0, "stabilization re-check interval in seconds (saved to quill.toml)")
	maxDelaysFlag := flag.Int("max-delays", 0, "max consecutive delays before forced commit (saved to quill.toml)")
	presetFlag := flag.String("preset", "", "timing preset: active (default), deep, aggressive")
	strategyFlag := flag.String("strategy", "", "commit strategy: permissive, standard (default), strict")
	versionFlag := flag.Bool("version", false, "print version and exit")
	pathFlag := flag.String("path", "", "add or remove quill-commit from PATH: --path add, --path remove")
	delaysFlag := flag.String("delays", "", "enable or disable model delay requests: --delays enable, --delays disable")

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
	c.Strategy = *strategyFlag
	c.Version = *versionFlag
	c.PathAction = strings.ToLower(*pathFlag)
	c.DelaysAction = strings.ToLower(*delaysFlag)

	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "interval":
			c.IntervalSet = true
		case "stabilize":
			c.StabilizeSet = true
		case "max-delays":
			c.MaxDelaysSet = true
		case "model":
			c.ModelSet = true
		case "strategy":
			c.StrategySet = true
		case "preset":
			c.PresetSet = true
		case "delays":
			c.DelaysSet = true
		}
	})

	return nil
}
