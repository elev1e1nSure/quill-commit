package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

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
