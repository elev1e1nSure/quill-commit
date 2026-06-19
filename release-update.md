## ✨ Features

**quill-commit** — AI-powered auto-commit tool for Git. Watches your repo and automatically commits changes when the AI decides a logical unit of work is complete.

- **AI-driven commits** — integrates with OpenRouter (default: DeepSeek V4 Flash) to analyze diffs and generate Conventional Commit messages
- **Smart stabilization** — buffers rapid edits, only commits when the diff stops changing
- **Three presets** — `active` (commits fast), `deep` (waits longer for bigger chunks), `aggressive` (near-real-time)
- **TUI interface** — real-time status panel with next-check timer, delay counter, and scrollable event log
- **Persistent config** — `quill.toml` in the repo root, CLI flags override and persist
- **API key management** — flag, env var (`OPENROUTER_API_KEY`), or auto-saved credentials file
- **Conventional Commits** — all generated messages follow the standard, enforced by commitlint + husky

## 🐛 Fixes

- Security: `GITHUB_TOKEN` now has explicit `contents: write` scope for release creation
- Race conditions: `doCommit` re-checks diff before `git add`, atomic config file creation
- Error handling: network errors don't count toward max delays, stderr captured in git errors
- Stabilization: separate cadence for stabilize re-checks, reset delay counter on diff change during sleep

## 🔧 Maintenance

- Full test coverage: watcher (mock git/ai), config (file-based edge cases), AI (network errors, empty diffs)
- Go 1.24.2, golangci-lint, husky pre-commit hook runs lint + tests