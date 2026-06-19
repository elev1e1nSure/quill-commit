# CLAUDE.md

## Project

quill-commit — a Go TUI tool that watches a git repo and auto-commits changes via OpenRouter AI on a timer. Takes `--api-key`, runs a ticker, sends stabilized diffs to an LLM, and commits when the model approves.

## Stack

- **Language:** Go 1.24.2
- **TUI:** `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/lipgloss` + `github.com/charmbracelet/bubbles`
- **Config:** `github.com/pelletier/go-toml/v2`
- **Lint:** `golangci-lint`
- **Commits:** `commitlint` + `husky` (conventional commits)

## Code style

- Standard Go conventions: `gofmt`, `go vet`, `golangci-lint` clean.
- Package layout: `config/`, `git/`, `ai/`, `watcher/`, `ui/`.
- Zero external logging libraries — `fmt.Fprint` to bubbletea model or stderr.
- Errors are values, never panic.
- HTTP client with sensible timeouts (10s connect, 30s read).

## Docs

- `docs/architecture.md` — design decisions, watcher logic, event types, package layout
- `docs/development.md` — build/test/lint commands, tooling setup, commit conventions

## Rules

- Read `docs/architecture.md` before starting any feature work.
- Only conventional commits: `type(scope): description` (types: feat, fix, chore, docs, style, refactor, perf, test, ci, build).
- No emoji in code or commits.
- Do not commit secrets, `.env` files, or `quill.toml` with real keys.
- TUI minimal — two blocks (Status + Log), verbose block reserved but hidden in MVP.