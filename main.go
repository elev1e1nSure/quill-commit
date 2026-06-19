package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"quill-commit/config"
	"quill-commit/credentials"
	"quill-commit/git"
	"quill-commit/ui"
	"quill-commit/watcher"
)

var (
	stTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C9BD2")).Bold(true)
	stFlag    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4D4D4"))
	stMeta    = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	stErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("#D44A4A"))
)

func printUsage() {
	exe := "quill-commit"
	lines := []string{
		stTitle.Render("Usage") + ": " + exe + " [options]",
		"",
		stTitle.Render("Options"),
		"  " + stFlag.Render("-api-key") + " string    OpenRouter API key (saved for future runs)",
		"  " + stFlag.Render("-preset") + " string    active (default), deep, aggressive",
		"  " + stFlag.Render("-model") + " string    model override (overrides quill.toml)",
		"  " + stFlag.Render("-interval") + " float    check interval in minutes (overrides quill.toml)",
		"  " + stFlag.Render("-stabilize") + " float   stabilization re-check interval in minutes (overrides quill.toml)",
		"  " + stFlag.Render("-max-delays") + " int     max delays before forced commit (overrides quill.toml)",
		"",
		stTitle.Render("Presets"),
		"  " + stFlag.Render("active") + "      " + stMeta.Render("interval=2m  stabilize=1m   max_delays=3  — active coding sessions (default)"),
		"  " + stFlag.Render("deep") + "        " + stMeta.Render("interval=5m  stabilize=2.5m max_delays=2  — long focused work"),
		"  " + stFlag.Render("aggressive") + "  " + stMeta.Render("interval=30s stabilize=15s  max_delays=4  — frequent commits"),
		"",
		stMeta.Render("alternatively set QUILL_API_KEY env var"),
	}
	fmt.Println(strings.Join(lines, "\n"))
}

func main() {
	flag.CommandLine.Init(flag.CommandLine.Name(), flag.ContinueOnError)
	flag.CommandLine.SetOutput(&strings.Builder{})

	apiKeyFlag := flag.String("api-key", "", "OpenRouter API key (saved to credentials file for future runs)")
	modelFlag := flag.String("model", "", "model override (overrides quill.toml)")
	intervalFlag := flag.Float64("interval", 0, "check interval in minutes (overrides quill.toml)")
	stabilizeFlag := flag.Float64("stabilize", 0, "stabilization re-check interval in minutes (overrides quill.toml)")
	maxDelaysFlag := flag.Int("max-delays", 0, "max consecutive delays before forced commit (overrides quill.toml)")
	presetFlag := flag.String("preset", "", "config preset: active (default), deep, aggressive")

	if len(os.Args) == 1 {
		printUsage()
		os.Exit(0)
	}

	err := flag.CommandLine.Parse(os.Args[1:])
	if err != nil {
		if err == flag.ErrHelp {
			printUsage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "%s %s\n\n", stErr.Render("error:"), err.Error())
		printUsage()
		os.Exit(1)
	}

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
	if *presetFlag != "" {
		if !config.ApplyPreset(&cfg, *presetFlag) {
			fmt.Fprintf(os.Stderr, "error: unknown preset %q — valid presets: active, deep, aggressive\n", *presetFlag)
			os.Exit(1)
		}
		dirty = true
	}
	if *modelFlag != "" {
		cfg.Model = *modelFlag
		dirty = true
	}
	if *intervalFlag > 0 {
		cfg.Interval = *intervalFlag
		dirty = true
	}
	if *stabilizeFlag > 0 {
		cfg.Stabilize = *stabilizeFlag
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
