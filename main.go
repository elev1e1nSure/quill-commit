package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"quill-commit/config"
	"quill-commit/git"
	"quill-commit/ui"
	"quill-commit/watcher"
)

func main() {
	apiKey := flag.String("api-key", "", "OpenRouter API key")
	model := flag.String("model", "", "model override (overrides quill.toml)")
	flag.Parse()

	if *apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: --api-key is required")
		os.Exit(1)
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

	if *model != "" {
		cfg.Model = *model
	}

	w := watcher.New(cfg, *apiKey)
	go w.Run()

	p := tea.NewProgram(ui.New(cfg, w.Events), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
