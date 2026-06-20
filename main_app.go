package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"quill-commit/config"
	"quill-commit/ui"
	"quill-commit/watcher"
)

// App wires the watcher and the TUI together and runs them.
type App struct {
	cfg      config.Config
	apiKey   string
	repoRoot string
}

// Run creates the watcher, starts it, and runs the Bubble Tea program.
func (a *App) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := watcher.New(ctx, a.cfg, a.apiKey, a.repoRoot)
	defer w.Close()
	go w.Run()

	p := tea.NewProgram(ui.New(a.cfg, w.Events, w.Cmds), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
