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
		stTitle.Render("Usage") + ": " + exe + " [flags]",
		"",
		stTitle.Render("Flags"),
		"  " + stFlag.Render("--api-key") + " string      OpenRouter API key (saved to credentials file)",
		"  " + stFlag.Render("--model") + " string         LLM model to use (saved to quill.toml)",
		"  " + stFlag.Render("--preset") + " string        timing preset: active (default), deep, aggressive",
		"  " + stFlag.Render("--strategy") + " string      commit strategy: permissive, standard (default), strict",
		"  " + stFlag.Render("--interval") + " float       check interval in minutes",
		"  " + stFlag.Render("--stabilize") + " float      stabilization re-check interval in minutes",
		"  " + stFlag.Render("--max-delays") + " int       max delays before forced commit (0 = never force)",
		"  " + stFlag.Render("--configure") + "            save settings to quill.toml and exit without starting",
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
		stMeta.Render("API key: --api-key flag → QUILL_API_KEY env → credentials file"),
		stMeta.Render("All flags except --api-key, --configure, --version are saved to quill.toml"),
	}
	fmt.Println(strings.Join(lines, "\n"))
}

func printConfigured(cfg config.Config) {
	lines := []string{
		stTitle.Render("quill-commit configured"),
		"",
		"  " + stFlag.Render("model") + "       " + cfg.Model,
		"  " + stFlag.Render("strategy") + "    " + cfg.Strategy,
		"  " + stFlag.Render("interval") + "    " + fmt.Sprintf("%.4gm", cfg.Interval),
		"  " + stFlag.Render("stabilize") + "   " + fmt.Sprintf("%.4gm", cfg.Stabilize),
		"  " + stFlag.Render("max-delays") + "  " + fmt.Sprintf("%d", cfg.MaxDelays),
	}
	fmt.Println(strings.Join(lines, "\n"))
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

	// --configure: save settings and exit without starting the watcher.
	if cli.Configure {
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

	creds := &CredentialResolver{FlagValue: cli.APIKey}
	apiKey, shouldSave, err := creds.Resolve()
	if err != nil {
		return err
	}
	if apiKey == "" {
		return fmt.Errorf("API key required — pass --api-key, set QUILL_API_KEY, or run once with --api-key to save it")
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
