# quill-commit

**AI-powered auto-commits.** Watches your repo, waits for your changes to stabilize, asks a model if the diff makes sense as a commit — and commits it with a proper message.

[![Release](https://img.shields.io/github/v/release/elev1e1nSure/quill-commit?style=flat-square&color=6C9BD2)](https://github.com/elev1e1nSure/quill-commit/releases)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License](https://img.shields.io/github/license/elev1e1nSure/quill-commit?style=flat-square&color=808080)](LICENSE)

---

![demo](docs/demo.png)

---

## What it does

You write code. quill-commit watches `git diff`, waits for the pace to slow down, then asks an LLM: *"is this a coherent unit of work?"* If yes — it commits with a generated [Conventional Commit](https://www.conventionalcommits.org) message. If the diff contains several independent changes belonging to different scopes (e.g. a bugfix in one package and unrelated docs in another), it will split them into sequential, atomic commits. If no — it waits a bit and tries again. After too many noes, it force-commits so nothing ever gets lost.

No heuristics. No line-count thresholds. Just the model looking at your actual diff.

## Install

```sh
go install github.com/elev1e1nSure/quill-commit@latest
```

Or build from source:

```sh
git clone https://github.com/elev1e1nSure/quill-commit
cd quill-commit
just build
```

## Quickstart

Get an API key from [openrouter.ai](https://openrouter.ai), then:

```sh
quill-commit --api-key <your-key>
```

The key is saved for future runs. Next time just:

```sh
quill-commit
```

## Presets

Pick a rhythm that matches how you work:

| Preset | Checks every | For |
|--------|-------------|-----|
| `active` *(default)* | 2 min | Normal coding sessions |
| `deep` | 5 min | Long focused work, big refactors |
| `aggressive` | 30 sec | Fast feedback loops |

```sh
quill-commit --preset aggressive
```

## TUI Controls & Hotkeys

While `quill-commit` is running, you can control it interactively from the TUI:

- **`p`**: Pause/resume the watcher. When paused, the status displays a red `PAUSED` indicator, and no checks will run.
- **`a`**: Manually trigger an AI-assisted commit amendment. It takes your current staged and unstaged changes, retrieves the last commit message, sends both to the LLM to write a combined message, and runs `git commit --amend`.
- **`q` / `ctrl+c`**: Quit the application. Requires a confirmation double-press within 3 seconds to prevent accidental exits.

## Configuration

Settings are stored in `quill.toml` in your repo root and created automatically on first run.

```toml
model      = "deepseek/deepseek-v4-flash"
interval   = 2    # minutes between checks
stabilize  = 1    # re-check cadence while diff is still changing
max_delays = 0    # force-commit after this many consecutive delays (0 to disable)
```

All flags override `quill.toml` and are saved back to it.

### CLI Flags

The following flags can be used to customize execution:
- `--api-key <key>`: OpenRouter API key.
- `--preset <preset>`: Config preset (`active`, `deep`, `aggressive`).
- `--model <name>`: LLM model to query.
- `--interval <mins>`: Interval between checks.
- `--stabilize <mins>`: Stabilization delay.
- `--max-delays <count>`: Max delays before forced commit (0 to disable).
- `--version`: Print version information and exit.

## API key resolution

First match wins:

1. `--api-key` flag
2. `QUILL_API_KEY` env var
3. Credentials file (`~/.config/quill-commit/credentials` on Linux/macOS, `%APPDATA%\quill-commit\credentials` on Windows)

## Docs

- [Architecture](docs/architecture.md) — how the watcher, stabilization, and LLM loop work
- [Development](docs/development.md) — build, test, lint
