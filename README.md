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
quill-commit --api-key <openrouter-key> [--model <model-id>]
```

Reads `quill.toml` from the current directory. Creates it with defaults on first run.

| Flag | Description |
|------|-------------|
| `--api-key` | OpenRouter API key. Required. |
| `--model` | Model override. Takes precedence over `quill.toml`. |

**Quit:** `q` or `Ctrl+C` in the TUI.

## Config (`quill.toml`)

```toml
interval = 10                          # how often to check, in minutes
max_delays = 3                         # forced commit after this many consecutive delays
model = "deepseek/deepseek-v4-flash"   # default model via OpenRouter
```

## How it works

1. Every `interval` minutes, captures `git diff HEAD`
2. Skips if diff is empty or changed since last tick (not stable yet)
3. Once the diff is the same two ticks in a row, sends it to the model
4. Model returns `commit: true/false`, a suggested delay, and a message
5. On `commit: true` → `git add -A && git commit -m "<message>"`
6. On `commit: false` → waits the suggested delay, retries
7. After `max_delays` consecutive delays → force-commits with `auto: forced commit`
8. Network errors don't count toward the delay counter

## Docs

- [Architecture](docs/architecture.md) — design decisions and tradeoffs
- [Development](docs/development.md) — build, test, lint, tooling
