# Development

## Requirements

- Go 1.24.2+
- [just](https://github.com/casey/just) task runner
- [golangci-lint](https://golangci-lint.run)
- Node.js (for commitlint + husky hooks)

## Setup

```
npm install   # installs commitlint + husky
just build
```

## Commands

```
just build         # compile binary
just test          # go test ./...
just lint          # golangci-lint run ./...
just tidy          # go mod tidy
just commit        # git add -A && commit
just release-notes # go run ./cmd/releasenotes (run with --from=v1.0.0 --to=HEAD)
```

## Commit conventions

Enforced by husky pre-commit + commit-msg hooks:

```
type(scope): description
```

Types: `feat`, `fix`, `chore`, `docs`, `style`, `refactor`, `perf`, `test`, `ci`, `build`

## Linting

Config in `.golangci.yml`. Enabled linters: `errcheck`, `govet`, `staticcheck`.

`errcheck` checks unchecked error returns (including blank assignments with `check-blank`).

## Testing

Each package has its own `_test.go`. The watcher uses interfaces (`gitOps`, `aiOps`) for dependency injection so git and AI calls can be mocked without spawning real processes.

The `git/` package contains low-level helpers (`Diff`, `Add`, `AddPaths`, `Commit`, `IsRepo`, `HeadHash`, `HeadMessage`, `AmendCommit`, `RecentCommits`, `StatusShort`, `LsFiles`, `RepoRoot`) that interact with git via subprocesses. Tests for these are performed using temporary git repositories.

The `credentials/` package reads/writes the API key to `~/.config/quill-commit/credentials`.

The `context/` package builds the static and dynamic project prompts. Its tests inject stub functions (like `lsFilesFunc`, `recentCommitsFunc`, and `statusShortFunc`) to test prompt formatting and budget truncation logic without subprocess overhead.

The `releasenotes/` package generates AI-powered release notes from a git range. Tested with mock HTTP.

The `watcher/` package is tested with a mocked `gitOps` and `aiOps` to verify behavior under various scenarios:
- Normal ticks, stabilization delays, and force-commits.
- Cache budget modifications (dynamic shrinkage on consecutive cache misses, and full restoration on hits).
- Failures/graceful degradation (e.g. static/dynamic context failures).
- Split commits handling and sweep/fallback operations.
- Interactive user commands (pausing, resuming, and manual AI-assisted amend mode).
- Structured slog event logging (verified to be bypassed in unit tests to avoid locking `log.txt` on Windows).

## Release builds

Builds are orchestrated via GitHub Actions (`.github/workflows/release.yml`):
- Cross-compiles for Linux, macOS, and Windows (`amd64`).
- Injects version metadata dynamically using `-ldflags="-X main.version=${GITHUB_REF_NAME}"` during `go build`.
