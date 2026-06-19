# quill-commit

Auto-commits your work using an LLM. Watches `git diff`, waits for changes to stabilize, asks a model whether the diff is a logical unit worth committing, then commits it with a generated message.

## Install

```
go install quill-commit@latest
```

Or build from source:

```
just build
```

## Usage

```
quill-commit [--api-key <key>] [--model <id>] [--interval <minutes>] [--stabilize <minutes>] [--max-delays <n>]
```

Reads `quill.toml` from the current directory. Creates it with defaults on first run.

**API key resolution** (first match wins):
1. `--api-key` flag — also saves the key to the credentials file for future runs
2. `QUILL_API_KEY` environment variable
3. Credentials file (`~/.config/quill-commit/credentials` on Linux/macOS, `%APPDATA%\quill-commit\credentials` on Windows)

| Flag | Description |
|------|-------------|
| `--api-key` | OpenRouter API key. Saved to credentials file on use. |
| `--preset` | Apply a named config preset (saved to `quill.toml`). |
| `--model` | Model override. Saved to `quill.toml`. |
| `--interval` | How often to check for changes, in minutes. Supports decimals (`0.5` = 30s). Saved to `quill.toml`. |
| `--stabilize` | Re-check interval during stabilization, in minutes. Defaults to `interval / 2`. Saved to `quill.toml`. |
| `--max-delays` | Max consecutive delays before forced commit. Saved to `quill.toml`. |

## Presets

| Preset | interval | stabilize | max_delays | When to use |
|--------|----------|-----------|------------|-------------|
| `active` | 2m | 1m | 3 | Active coding sessions — default |
| `deep` | 5m | 2.5m | 2 | Long focused work, big refactors |
| `aggressive` | 30s | 15s | 4 | Fast feedback, frequent commits |

```
quill-commit --preset deep
```

Preset values are saved to `quill.toml` and persist across restarts.

**Quit:** `q` or `Ctrl+C` in the TUI.

## Config (`quill.toml`)

```toml
interval = 2                           # how often to check for changes, in minutes
stabilize = 1                          # re-check interval during stabilization (default: interval / 2)
max_delays = 3                         # forced commit after this many consecutive delays
model = "deepseek/deepseek-v4-flash"   # default model via OpenRouter
```

## How it works

1. Every `interval` minutes, captures `git diff HEAD`
2. Skips if diff is empty
3. If diff changed — waits `stabilize` minutes and re-checks until it stops changing
4. Once the diff is stable, sends it to the model
5. Model returns `commit: true/false`, a suggested delay, and a message
6. On `commit: true` → `git add -A && git commit -m "<message>"`
7. On `commit: false` → waits the suggested delay, retries
8. After `max_delays` consecutive delays → force-commits with `auto: forced commit`
9. Network errors don't count toward the delay counter

## Docs

- [Architecture](docs/architecture.md) — design decisions and tradeoffs
- [Development](docs/development.md) — build, test, lint, tooling
