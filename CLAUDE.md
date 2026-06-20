# CLAUDE.md

## Project

quill-commit — a Go TUI tool that watches a git repo and auto-commits changes via OpenRouter AI on a timer. Takes `--api-key`, runs a ticker, sends stabilized diffs to an LLM, and commits when the model approves.

## Stack

- **Language:** Go 1.24.2
- **TUI:** `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/lipgloss` + `github.com/charmbracelet/bubbles`
- **Config:** `github.com/pelletier/go-toml/v2`
- **Glob matching:** `github.com/bmatcuk/doublestar/v4` (for `.quillignore` path filtering)
- **Lint:** `golangci-lint` (errcheck, govet, staticcheck)
- **Commits:** `commitlint` + `husky` (conventional commits)

## Code style

- Standard Go conventions: `gofmt`, `go vet`, `golangci-lint` clean.
- Package layout: `credentials/`, `config/`, `pathfilter/`, `secretscan/`, `git/`, `context/`, `ai/`, `watcher/`, `ui/`, `releasenotes/`, `cmd/releasenotes/`.
- Main package split: `main.go` (entrypoint, usage), `main_cli.go` (CLI struct), `main_credentials.go` (credential resolver), `main_config.go` (config resolver), `main_app.go` (App struct).
- Zero external logging libraries — standard library `log/slog` for structured logging to `log.txt`, and `fmt.Fprint` to bubbletea model or stderr.
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
- TUI features: Status + Log blocks, footer hints. Interactivity: `p` pause/resume, `a` manual AI-amend, `q`/`ctrl+c` double-press quit confirmation, `ctrl+o` detail toggle for blocked commits.
- Events: EventCheck, EventSending, EventDecision, EventCommit, EventAmend, EventForced, EventSkip, EventDelay, EventError, EventInfo, EventCommitError, EventErrorExplain.
- Release notes: `cmd/releasenotes/` subcommand — `go run ./cmd/releasenotes --from=v1.0.0 --to=HEAD`.
- API key resolution: `--api-key` > `QUILL_API_KEY` env > credentials file.
- Credentials file path: `~/.config/quill-commit/credentials` (Linux/macOS), `%APPDATA%\quill-commit\credentials` (Windows).
