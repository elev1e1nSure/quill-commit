package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"quill-commit/config"
	"quill-commit/credentials"
	"quill-commit/git"
	"quill-commit/ui"
	"quill-commit/watcher"
)

func main() {
	apiKeyFlag := flag.String("api-key", "", "OpenRouter API key (saved to credentials file for future runs)")
	modelFlag := flag.String("model", "", "model override (overrides quill.toml)")
	intervalFlag := flag.Float64("interval", 0, "check interval in minutes (overrides quill.toml)")
	maxDelaysFlag := flag.Int("max-delays", 0, "max consecutive delays before forced commit (overrides quill.toml)")
	flag.Parse()

	apiKey := resolveAPIKey(*apiKeyFlag)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: API key required — pass --api-key, set QUILL_API_KEY, or run once with --api-key to save it")
		os.Exit(1)
	}
	if *apiKeyFlag != "" {
		if err := credentials.Save(*apiKeyFlag); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not save api key: %v\n", err)
		}
	}

	if !git.IsRepo() {
		fmt.Fprintln(os.Stderr, "error: not a git repository")
		os.Exit(1)
	}

	cfg, created, err := config.EnsureDefault(config.FileName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if created {
		fmt.Printf("created %s with defaults\n", config.FileName)
	}

	dirty := false
	if *modelFlag != "" {
		cfg.Model = *modelFlag
		dirty = true
	}
	if *intervalFlag > 0 {
		cfg.Interval = *intervalFlag
		dirty = true
	}
	if *maxDelaysFlag > 0 {
		cfg.MaxDelays = *maxDelaysFlag
		dirty = true
	}
	if dirty {
		if err := config.Save(config.FileName, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not save config: %v\n", err)
		}
	}

	w := watcher.New(cfg, apiKey)
	go w.Run()

	p := tea.NewProgram(ui.New(cfg, w.Events), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// resolveAPIKey returns the first non-empty value from:
// --api-key flag → QUILL_API_KEY env → credentials file.
func resolveAPIKey(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("QUILL_API_KEY"); env != "" {
		return env
	}
	stored, err := credentials.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: could not read credentials: %v\n", err)
		return ""
	}
	return stored
}
