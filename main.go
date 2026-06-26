package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"quill-commit/config"
	"quill-commit/credentials"
)

var version = "dev"

var (
	stTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C9BD2")).Bold(true)
	stFlag  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4D4D4"))
	stMeta  = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
)

func printUsage() {
	exe := "quill-commit"
	lines := []string{
		stTitle.Render("Usage") + ": " + exe + "             start watching and auto-committing",
		"       " + exe + " [flags]     configure settings (does not start the watcher)",
		"",
		stTitle.Render("Flags"),
		"  " + stFlag.Render("--api-key") + " string      OpenRouter API key (saved to credentials file)",
		"  " + stFlag.Render("--model") + " string         LLM model to use (saved to quill.toml)",
		"  " + stFlag.Render("--preset") + " string        timing preset: active (default), deep, aggressive",
		"  " + stFlag.Render("--strategy") + " string      commit strategy: permissive, standard (default), strict",
		"  " + stFlag.Render("--interval") + " int         check interval in seconds",
		"  " + stFlag.Render("--stabilize") + " int        stabilization re-check interval in seconds",
		"  " + stFlag.Render("--max-delays") + " int       max delays before forced commit (3 by default)",
		"  " + stFlag.Render("--delays") + " enable|disable  enable or disable model delay requests",
		"  " + stFlag.Render("--path") + " add|remove     add or remove quill-commit from user PATH",
		"  " + stFlag.Render("--version") + "              print version and exit",
		"",
		stTitle.Render("Timing presets"),
		"  " + stFlag.Render("active") + "      " + stMeta.Render("interval=2m  stabilize=1m   max_delays=3  — active coding (default)"),
		"  " + stFlag.Render("deep") + "        " + stMeta.Render("interval=5m  stabilize=2.5m max_delays=2  — long focused work"),
		"  " + stFlag.Render("aggressive") + "  " + stMeta.Render("interval=30s stabilize=15s  max_delays=4  — frequent commits"),
		"",
		stTitle.Render("Commit strategies"),
		"  " + stFlag.Render("standard") + "    " + stMeta.Render("commit complete, reasonable units of work (default)"),
		"  " + stFlag.Render("permissive") + "  " + stMeta.Render("commit everything that passes filters, no quality gating"),
		"  " + stFlag.Render("strict") + "      " + stMeta.Render("commit only clean, atomic, purposeful changes"),
		"",
		stMeta.Render("Run without flags to start. Flags configure and exit — they never start the watcher."),
		stMeta.Render("API key: --api-key → QUILL_API_KEY env → credentials file. Other flags saved to quill.toml."),
	}
	fmt.Println(strings.Join(lines, "\n"))
}

func printConfigured(cfg config.Config) {
	delaysState := "enable"
	if !cfg.Delayed() {
		delaysState = "disable"
	}
	presetLine := ""
	if cfg.Preset != "" {
		presetLine = "\n  " + stFlag.Render("preset") + "      " + cfg.Preset
	}
	lines := []string{
		stTitle.Render("quill-commit configured") + presetLine,
		"",
		"  " + stFlag.Render("model") + "       " + cfg.Model,
		"  " + stFlag.Render("strategy") + "    " + cfg.Strategy,
		"  " + stFlag.Render("interval") + "    " + formatSec(cfg.Interval),
		"  " + stFlag.Render("stabilize") + "   " + formatSec(cfg.Stabilize),
		"  " + stFlag.Render("max-delays") + "  " + fmt.Sprintf("%d", cfg.MaxDelays),
		"  " + stFlag.Render("delays") + "      " + delaysState,
	}
	fmt.Println(strings.Join(lines, "\n"))
}

func formatSec(s int) string {
	if s >= 60 {
		m := s / 60
		rem := s % 60
		if rem == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, rem)
	}
	return fmt.Sprintf("%ds", s)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cli := &CLI{}
	if err := cli.Parse(); err != nil {
		printUsage()
		return err
	}
	if cli.ShowUsage {
		printUsage()
		return nil
	}
	if cli.Version {
		fmt.Println("quill-commit", version)
		return nil
	}

	if cli.PathAction != "" {
		return handlePath(cli.PathAction)
	}

	if cli.DelaysAction != "" {
		if cli.DelaysAction != "enable" && cli.DelaysAction != "disable" {
			return fmt.Errorf("--delays expects 'enable' or 'disable', got %q", cli.DelaysAction)
		}
	}

	// Any flags passed → configure and exit.
	if len(os.Args) > 1 {
		if cli.APIKey != "" {
			if err := credentials.Save(cli.APIKey); err != nil {
				fmt.Fprintf(os.Stderr, "warn: could not save api key: %v\n", err)
			}
		}
		cfgResolver := &ConfigResolver{CLI: *cli}
		cfg, _, err := cfgResolver.Resolve()
		if err != nil {
			return err
		}
		printConfigured(cfg)
		return nil
	}

	// No flags → start watcher.
	creds := &CredentialResolver{FlagValue: cli.APIKey}
	apiKey, shouldSave, err := creds.Resolve()
	if err != nil {
		return err
	}
	if apiKey == "" {
		return fmt.Errorf("API key required — run quill-commit --api-key <key> first")
	}
	if shouldSave {
		if err := credentials.Save(cli.APIKey); err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not save api key: %v\n", err)
		}
	}

	cfgResolver := &ConfigResolver{CLI: *cli}
	cfg, repoRoot, err := cfgResolver.Resolve()
	if err != nil {
		return err
	}

	app := &App{cfg: cfg, apiKey: apiKey, repoRoot: repoRoot}
	return app.Run()
}
