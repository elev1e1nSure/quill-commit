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
just build    # compile binary
just test     # go test ./...
just lint     # golangci-lint run ./...
just tidy     # go mod tidy
just commit   # git add -A && commit
```

## Commit conventions

Enforced by husky pre-commit + commit-msg hooks:

```
type(scope): description
```

Types: `feat`, `fix`, `chore`, `docs`, `style`, `refactor`, `perf`, `test`, `ci`, `build`

## Linting

Config in `.golangci.yml`. Enabled linters: `errcheck`, `govet`, `staticcheck`.

## Testing

Each package has its own `_test.go`. The watcher uses interfaces (`gitOps`, `aiOps`) for dependency injection so git and AI calls can be mocked without spawning real processes.
