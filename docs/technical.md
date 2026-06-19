# quill-commit — Technical Plan

## Bootstrap

- [x] `go mod init quill-commit`
- [x] Add dependencies: bubbletea, lipgloss, bubbles, go-toml/v2
- [x] Scaffold package dirs: `config/`, `git/`, `ai/`, `watcher/`, `ui/`
- [x] Create `main.go` entry point (flag parsing, startup checks, launch TUI)

## config/

- [x] Define `Config` struct (`interval`, `max_delays`, `model`)
- [x] Read `quill.toml` from cwd; create with defaults if missing

## git/

- [x] `Diff() (string, error)` — runs `git diff HEAD`, returns output
- [x] `Add() error` — runs `git add -A`
- [x] `Commit(message string) error` — runs `git commit -m`
- [x] Startup check: `git rev-parse --git-dir` → exit if not a git repo

## ai/

- [x] `Decision` struct: `Commit bool`, `Delay int`, `Message string`
- [x] `Ask(diff, model, apiKey string) (Decision, error)` — POST to OpenRouter
- [x] HTTP client with 10s dial timeout, 30s response timeout
- [x] Parse JSON response; invalid JSON → return `Decision{Commit: true, Message: "auto: fallback commit"}`

## watcher/

- [x] `Watcher` struct: holds config, prev diff, delay counter, ticker
- [x] Ticker loop every `interval` minutes
- [x] Stabilization: skip if diff empty; skip if diff changed; proceed if diff same as prev tick
- [x] On model `commit: false`: wait `delay` minutes, retry model (not next tick)
- [x] Delay counter: increment on `commit: false`; reset on commit; skip on network error
- [x] `max_delays` hit → force commit `"auto: forced commit"`, reset counter
- [x] Emit events to TUI via channel

## ui/

- [ ] Bubbletea `Model` with `Status` and `Log` blocks
- [ ] Status block: interval, time to next check, delay counter, last commit hash+message
- [ ] Log block: timestamped entries (check, model decision, commit, error)
- [ ] Lipgloss styles per color spec (accent1 `#6C9BD2`, accent2 `#D4842A`, text `#D4D4D4`, dim `#808080`, success `#5FA862`, warn `#D4A82A`, error `#D44A4A`)
- [ ] Receive watcher events via channel, append to log, re-render
- [ ] Verbose block: reserved in layout, hidden (no toggle in MVP)

## main.go

- [ ] Parse `--api-key` and `--model` flags
- [ ] Validate: missing `--api-key` → exit with error
- [ ] Load/create config
- [ ] Run startup checks (git repo)
- [ ] Start watcher in goroutine
- [ ] Start bubbletea program, block until quit

## Tooling

- [x] `justfile` with `build`, `run`, `lint`, `test`, `tidy`
- [x] `golangci-lint` installed
- [x] `commitlint` + `husky` pre-commit and commit-msg hooks
- [x] `.golangci.yml` config (enable `errcheck`, `govet`, `staticcheck`)
- [x] `just build` produces working binary

---

## Done

- [x] ~~`go mod init quill-commit`~~
- [x] ~~Add dependencies: bubbletea, lipgloss, bubbles, go-toml/v2~~
- [x] ~~Scaffold package dirs: `config/`, `git/`, `ai/`, `watcher/`, `ui/`~~
- [x] ~~Create `main.go` entry point~~
- [x] ~~Define `Config` struct (`interval`, `max_delays`, `model`)~~
- [x] ~~Read `quill.toml` from cwd; create with defaults if missing~~
- [x] ~~`.golangci.yml` config~~
- [x] ~~`just build` produces working binary~~
- [x] ~~`git Diff()`, `Add()`, `Commit()`, `IsRepo()`~~
- [x] ~~`ai Decision` struct, `Ask()`, fallback~~
