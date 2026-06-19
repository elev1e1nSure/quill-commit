# quill-commit — Spec

## Purpose

A Go TUI tool that runs in a git repo directory and auto-commits changes on a timer via AI.

---

## Launch

```
quill-commit [--api-key <key>] [--model <model>]
```

Reads `quill.toml` from the current directory. If missing — creates with defaults and exits asking to pass `--api-key`.
Priority for api_key: `--api-key` flag only. Without it — error.
Priority for model: `--model` flag > `quill.toml`. If neither — default `deepseek/deepseek-v4-flash`.

---

## Config `quill.toml`

```toml
interval = 10       # ticker period in minutes
max_delays = 3      # max consecutive delays before forced commit
model = "deepseek/deepseek-v4-flash"  # fallback, overridden by --model flag
```

---

## Watcher logic

1. Ticker every `interval` minutes captures `git diff HEAD`
2. If diff is empty — wait for next tick
3. If diff is same as previous tick — send to model (changes stabilized)
4. If diff changed since previous tick — store new diff, wait for next tick
5. Model returns JSON:

```json
{"commit": true, "delay": 5, "message": "feat: add auth"}
```

6. If `commit: true` — `git add -A` + `git commit -m "..."`
7. If `commit: false` — wait `delay` minutes, then retry model
8. If `max_delays` consecutive delays — force commit without model with message `auto: forced commit`
9. After commit — reset delay counter and stored diff
10. Network errors — skip tick, log, **do not count** toward delay counter

---

## Model request

Single HTTP request to OpenRouter, minimal prompt:

```
You are an automatic git committer.
You receive a git diff. Decide if a logical unit of work is complete.
Return ONLY json without markdown:
{"commit": bool, "delay": int (minutes if commit false), "message": string (if commit true)}
```

Request body — diff only, nothing extra.

---

## TUI

Bubbletea + lipgloss, two blocks:

- **Status** — current interval, time until next check, delay counter, last commit
- **Log** — timestamped events: check, model decision, commit, errors

`/verbose` — toggle shows diff in a separate block below the log. **In MVP: block reserved in layout but hidden. Toggle not implemented.**

### Colors

| Role       | Hex       | Purpose                     |
|------------|-----------|-----------------------------|
| Accent 1   | `#6C9BD2` | Block headers               |
| Accent 2   | `#D4842A` | Status badges, commits      |
| Text       | `#D4D4D4` | Body text                   |
| Dim        | `#808080` | Field labels, timestamps    |
| Success    | `#5FA862` | commit: true, commit created |
| Warn       | `#D4A82A` | commit: false, delay         |
| Error      | `#D44A4A` | Network / parse errors       |

---

## Structure

```
quill-commit/
├── main.go           # entry point, config init, tui launch
├── config/
│   └── config.go     # read/write quill.toml
├── git/
│   └── git.go        # diff, add, commit
├── ai/
│   └── ai.go         # OpenRouter request, JSON parsing
├── watcher/
│   └── watcher.go    # ticker, stabilization logic, delays, orchestration
└── ui/
    └── ui.go         # bubbletea tui
```

---

## Tooling

- **Go 1.22** — language
- **bubbletea + lipgloss + bubbles** — TUI
- **pelletier/go-toml/v2** — `quill.toml` parsing
- **golangci-lint** — linting
- **commitlint + husky** — conventional commit enforcement
- **just** — task runner for all dev commands

---

## Error handling

- No internet / OpenRouter unreachable — skip tick, log, do not increment delay counter
- Model returned invalid JSON — force commit with `auto: fallback commit`
- Git not initialized in current directory — exit with error at startup
- `--api-key` not passed — exit with error at startup
- `quill.toml` looked up in current working directory. Not found — create there. No recursive search.
