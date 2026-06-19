# quill-commit

**AI-powered auto-commits.** Watches your repo, waits for your changes to stabilize, asks a model if the diff makes sense as a commit — and commits it with a proper message.

[![Release](https://img.shields.io/github/v/release/elev1e1nSure/quill-commit?style=flat-square&color=6C9BD2)](https://github.com/elev1e1nSure/quill-commit/releases)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![License](https://img.shields.io/github/license/elev1e1nSure/quill-commit?style=flat-square&color=808080)](LICENSE)

---

<!-- demo gif goes here -->
<!-- ![demo](docs/demo.gif) -->

---

## What it does

You write code. quill-commit watches `git diff`, waits for the pace to slow down, then asks an LLM: *"is this a coherent unit of work?"* If yes — it commits with a generated [Conventional Commit](https://www.conventionalcommits.org) message. If no — it waits a bit and tries again. After too many noes, it force-commits so nothing ever gets lost.

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

## Configuration

Settings are stored in `quill.toml` in your repo root and created automatically on first run.

```toml
model      = "deepseek/deepseek-v4-flash"
interval   = 2    # minutes between checks
stabilize  = 1    # re-check cadence while diff is still changing
max_delays = 3    # force-commit after this many consecutive "not yet"s
```

All flags override `quill.toml` and are saved back to it.

## API key resolution

First match wins:

1. `--api-key` flag
2. `QUILL_API_KEY` env var
3. Credentials file (`~/.config/quill-commit/credentials` on Linux/macOS, `%APPDATA%\quill-commit\credentials` on Windows)

## Docs

- [Architecture](docs/architecture.md) — how the watcher, stabilization, and LLM loop work
- [Development](docs/development.md) — build, test, lint
