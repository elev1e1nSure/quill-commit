# Architecture

## Why it exists

Commit discipline degrades under pressure. Developers either commit too rarely (one giant dump at the end) or too often (noise: "fix", "wip", "asdf"). Both make history useless.

`quill-commit` takes the decision out of the loop. The LLM sees the actual diff and decides when a change is coherent — like a senior dev saying "okay, that's a unit, commit it."

## Watcher logic

```
tick (every interval)
 ├─ paused?       → skip, wait for next tick/command
 ├─ diff empty?   → skip, reset delay counter, wait for next tick
 └─ diff changed? → stabilization loop (sleep stabilize, re-check)
      └─ diff stable (same two checks in a row)? → send to model
              └─ commit: true  → 
              │    ├─ multiple commits (split)? → sequence: git add -- <files> && git commit
              │    └─ single commit?             → git add -- <IncludedFiles> && git commit
              └─ commit: false → 
                   ├─ max_delays hit (if > 0)?   → force commit
                   └─ else                       → sleep delay, retry model
```

After any commit (single, split, or forced), the delay counter and stored diff both reset.
The delay counter also resets to 0 if a git/AI error occurs or if a check is skipped because the diff is empty or has changed during a delay loop, preventing stale delay states.

### Commit-blocked retry

When a commit is rejected (e.g. pre-commit hook failure), the watcher stores a SHA-256 fingerprint of the stable diff in `blockedDiffHashes`. On subsequent ticks, if the diff hash matches, the tick is silently skipped — preventing spam. Once the user modifies the working tree (diff changes), normal operation resumes. The model is also asked to explain the error and a short summary + fix suggestion is shown in the TUI.

## Key design decisions

**Two-speed stabilization.** `interval` controls how often the watcher starts a fresh check when nothing is happening. Once a non-empty diff appears, the watcher switches to `stabilize` re-checks (typically `interval / 2`) until the diff is unchanged — only then does it send to the model. This matters most for the `aggressive` preset: a 30s interval with a 15s stabilize re-check catches the pause-between-bursts faster than waiting another 30s for the next ticker.

**LLM as the only oracle.** No heuristics, no line-count thresholds. The model sees the filtered diff — tracked changes plus untracked files after path and content filtering — and reasons about logical completeness. Invalid JSON from the model → fallback commit (`auto: fallback commit`).

**Security filters as intentional guardrails.** The three-layer secret filter (path filter, content scan, add filter) is a deliberate deviation from the "no heuristics" principle. Secrets are removed from the diff *before* the model sees them, and only files that pass both filters are ever staged. This prevents accidental leakage of credentials to the LLM and into git history. The model is never shown `.env`, `.pem`, `id_rsa`, or any file containing known token signatures (e.g. `sk-or-v1-`, `AKIA`, `ghp_`).

**Split Commits for atomic history.** If a large, stable diff contains several independent changes belonging to different scopes (e.g., a bugfix in one package and unrelated documentation in another), the model can return a list of commit groups. The watcher stages only the specified files for each group and commits them sequentially. Any remaining unassigned files are swept into a final `chore: commit remaining changes` commit.

**Network errors don't count.** A failed API call is not a delay. The counter only increments on explicit `commit: false`. This prevents network flakiness from burning through `max_delays`.

**Force-commit as a safety net.** `max_delays` (if set to > 0) ensures nothing is ever silently lost, even if the model keeps saying "not yet." If `max_delays = 0` (the default), forced commits are disabled, and the watcher will wait indefinitely for the model's approval.

**Delay loop vs. ticker.** When the model says wait, the watcher sleeps inline (not waiting for the next tick). This decouples model-suggested delays from the stabilization interval — a 5-minute delay doesn't have to align with a 10-minute tick.

**TUI Interactive Commands.** TUI users can send controls to the watcher:
- `p` pauses/resumes the ticker.
- `a` triggers a manual, AI-assisted commit amendment (adds current changes and amends the last commit with a merged message).
- `q` / `ctrl+c` quits safely (requires a double-press confirmation).
- `ctrl+o` toggles detail view for blocked commits (raw error + AI explanation).

**Structured Event Logging.** Logs are written to `log.txt` in the repository root using Go's standard library `log/slog`.
- To optimize disk and I/O operations, the log file is opened once at watcher startup and closed on shutdown.
- Log events map directly to standard severities (`DEBUG`, `INFO`, `WARN`, `ERROR`), permitting straightforward filtering.
- Logging is skipped in test runs to prevent OS file locks.

## Package layout

```
credentials/ API key persistence to ~/.config/quill-commit/credentials
config/      Config struct, quill.toml read/write, presets
pathfilter/  Hardcoded secret path patterns + .quillignore parser (doublestar)
secretscan/  Regex-based secret signature detection (API keys, tokens)
git/         Diff, DiffEx, Add, AddPaths, Commit, IsRepo, HeadHash, HeadMessage, AmendCommit, RecentCommits, StatusShort, LsFiles, RepoRoot
context/     BuildStatic, BuildDynamic, RenderSystem, RenderUser, Hash
ai/          OpenRouter request, Decision struct, fallback, CacheCapability
watcher/     Scheduler, stabilization, delay loop, context manager, commit engine, event logging, command handling, quarantine
ui/          Bubbletea TUI — Status block + Log block + footer hints
releasenotes/ AI-powered release note generation from git history
cmd/releasenotes/ CLI entrypoint for release notes (go run ./cmd/releasenotes)
main.go      Flag parsing, credential/config resolution, wire everything together
main_cli.go         CLI struct and Parse()
main_credentials.go CredentialResolver (flag → env → file)
main_config.go      ConfigResolver (load quill.toml, apply preset/overrides, persist)
main_app.go         App struct that starts Watcher + TUI
```

## Project context and prompt caching

To make commit decisions more accurate, `quill-commit` constructs a project context that is sent to the LLM. It supports prompt caching to minimize costs and latency.

### Context config
- `include_context` (default `true`): enable/disable context entirely.
- `context_budget` (default `32000`): max characters for static context before truncation (drops Conventions → Packages → truncates Stack).
- `recent_commits` (default `10`): how many past commit messages to include in dynamic context.

### Context Types
- **Static Context**: Extracted once at startup (via `ContextManager`).
  - Project description & Stack: Loaded from `CLAUDE.md`, falling back to `README.md`, then `AGENTS.md`.
  - Packages list: Top-level directories retrieved from `git ls-files`, sorted lexically.
  - Conventions: Hardcoded Conventional Commits guidelines.
- **Dynamic Context**: Re-evaluated on every check (via `BuildDynamic`).
  - Recent commits: A list of the latest `n` commit messages.
  - Changed files: Brief status of untracked and modified files (`git status --short`).

### Request Shape
- **System Prompt**: Built as `BasePrompt + "\n\n---\n\n" + RenderSystem(static, budget)`.
- **User Prompt**: Built as `RenderUser(dynamic) + "\n\n" + stableDiff`.
- **Session ID**: Generated per-run (crypto/rand, fallback to timestamp+pid) or overridden in TOML. Sent to OpenRouter to enable sticky routing, which triggers provider-side prompt caching.

### Cache Capability & Miss Budget
- At startup, `ContextManager` probes `GET https://openrouter.ai/api/v1/models/{model}` to verify if it supports the `cache_control` parameter.
- If supported, `ExplicitCache` is enabled, inserting `cache_control: {type: "ephemeral"}` blocks into the system message.
- To prevent runaway costs on continuous cache misses, the watcher tracks misses via `UpdateBudget(usage)`:
  - If 3 consecutive cache misses occur, the static context budget shrinks from the full value to 800 characters.
  - As soon as a cache hit is registered (`cached_tokens > 0`), the budget is restored to its full configured value.

### Failure Modes & Degradation
- If `BuildStatic` or `BuildDynamic` fails, the system logs a warning (to stderr and/or the event log) and degrades gracefully (omits the relevant context sections).
- If the `CacheCapability` probe fails, it defaults to disabling explicit caching without crashing.
- If `session_id` generation fails (crypto/rand), it falls back to a mix of `time.Now().UnixNano()` and `os.Getpid()`.

## Commit error handling

When `Commit` or `Split` fail (pre-commit hook rejects the commit), the watcher:
1. Stores the current stable diff in `commitBlockedDiff`.
2. Emits `EventCommitError` with the raw error text.
3. Optionally calls `AskExplain` — sends the error to the model with a explain-error prompt, then emits `EventErrorExplain` with a user-facing summary and fix.

The TUI shows `commit blocked  ctrl+o for details`; pressing `ctrl+o` toggles between raw error and AI explanation.

## TUI events

The watcher emits typed events to a buffered channel; the TUI consumes them via `Presenter`:

| Event | When |
|-------|------|
| `EventCheck` | Each tick starts |
| `EventSending` | About to send request to the model |
| `EventDecision` | Model responded (commit or wait) |
| `EventCommit` | Commit succeeded |
| `EventAmend` | Manual amend completed |
| `EventForced` | Max delays hit, forcing commit |
| `EventSkip` | Diff empty or changed during stabilization |
| `EventDelay` | About to sleep before retry |
| `EventError` | Git or AI error |
| `EventInfo` | Informational message (e.g. amend nothing, context warn) |
| `EventCommitError` | Pre-commit hook or script blocked the commit |
| `EventErrorExplain` | AI explanation of a commit failure |
