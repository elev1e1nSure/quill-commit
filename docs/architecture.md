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
              ├─ commit: true  → 
              │    ├─ multiple commits (split)? → sequence: git add <files> && git commit
              │    └─ single commit?             → git add -A && git commit
              └─ commit: false → 
                   ├─ max_delays hit (if > 0)?   → force commit
                   └─ else                       → sleep delay, retry model
```

After any commit (single, split, or forced), the delay counter and stored diff both reset.
The delay counter also resets to 0 if a git/AI error occurs or if a check is skipped because the diff is empty or has changed during a delay loop, preventing stale delay states.

## Key design decisions

**Two-speed stabilization.** `interval` controls how often the watcher starts a fresh check when nothing is happening. Once a non-empty diff appears, the watcher switches to `stabilize` re-checks (typically `interval / 2`) until the diff is unchanged — only then does it send to the model. This matters most for the `aggressive` preset: a 30s interval with a 15s stabilize re-check catches the pause-between-bursts faster than waiting another 30s for the next ticker.

**LLM as the only oracle.** No heuristics, no line-count thresholds. The model sees the full diff — tracked changes plus untracked files — and reasons about logical completeness. Invalid JSON from the model → fallback commit (`auto: fallback commit`).

**Split Commits for atomic history.** If a large, stable diff contains several independent changes belonging to different scopes (e.g., a bugfix in one package and unrelated documentation in another), the model can return a list of commit groups. The watcher stages only the specified files for each group and commits them sequentially. Any remaining unassigned files are swept into a final `chore: commit remaining changes` commit.

**Network errors don't count.** A failed API call is not a delay. The counter only increments on explicit `commit: false`. This prevents network flakiness from burning through `max_delays`.

**Force-commit as a safety net.** `max_delays` (if set to > 0) ensures nothing is ever silently lost, even if the model keeps saying "not yet." If `max_delays = 0` (the default), forced commits are disabled, and the watcher will wait indefinitely for the model's approval.

**Delay loop vs. ticker.** When the model says wait, the watcher sleeps inline (not waiting for the next tick). This decouples model-suggested delays from the stabilization interval — a 5-minute delay doesn't have to align with a 10-minute tick.

**TUI Interactive Commands.** TUI users can send controls to the watcher:
- `p` pauses/resumes the ticker.
- `a` triggers a manual, AI-assisted commit amendment (adds current changes and amends the last commit with a merged message).
- `q` / `ctrl+c` quits safely (requires a double-press confirmation).

## Package layout

```
config/    Config struct, quill.toml read/write
git/       Diff, Add, AddPaths, Commit, IsRepo, HeadHash, HeadMessage, AmendCommit, RecentCommits, StatusShort, LsFiles, RepoRoot
context/   BuildStatic, BuildDynamic, RenderSystem, RenderUser, Hash
ai/        OpenRouter request, Decision struct, fallback, CacheCapability
watcher/   Ticker, stabilization, delay loop, command handling (Cmds), event emission, context/caching integration
ui/        Bubbletea TUI — Status block + Log block + footer hints
main.go    Flag parsing, startup checks, wires everything together
```

## Project context and prompt caching

To make commit decisions more accurate, `quill-commit` constructs a project context that is sent to the LLM. It supports prompt caching to minimize costs and latency.

### Context Types
- **Static Context**: Extracted once at startup.
  - Project description & Stack: Loaded from `CLAUDE.md`, falling back to `README.md`, then `AGENTS.md`.
  - Packages list: Top-level directories retrieved from `git ls-files`, sorted lexically.
  - Conventions: Hardcoded Conventional Commits guidelines.
- **Dynamic Context**: Re-evaluated on every check.
  - Recent commits: A list of the latest `n` commit messages (default 10).
  - Changed files: Brief status of untracked and modified files (`git status --short`).

### Request Shape
- **System Prompt**: Built as `BasePrompt + "\n\n---\n\n" + RenderSystem(static, budget)`.
- **User Prompt**: Built as `RenderUser(dynamic) + "\n\n" + stableDiff`.
- **Session ID**: Generated per-run (or overridden in TOML) and sent to OpenRouter to enable sticky routing, which triggers provider-side prompt caching.

### Cache Capability & Miss Budget
- At startup, `quill-commit` probes `GET https://openrouter.ai/api/v1/models/{model}` to verify if it supports the `cache_control` parameter.
- If supported, `ExplicitCache` is enabled, inserting `cache_control: {type: "ephemeral"}` blocks into the system message.
- To prevent runaway costs on continuous cache misses, the watcher tracks misses:
  - If 3 consecutive cache misses occur, the static context budget shrinks to 800 characters.
  - As soon as a cache hit is registered, the budget is restored to its full configured value.

### Failure Modes & Degradation
- If `BuildStatic` or `BuildDynamic` fails, the system logs a warning to stderr and degrades gracefully (e.g. omitting the missing context sections or continuing with partial context).
- If the `CacheCapability` probe fails, it defaults to disabling explicit caching without crashing.

## TUI events

The watcher emits typed events to a buffered channel; the TUI consumes them:

| Event | When |
|-------|------|
| `EventCheck` | Each tick starts |
| `EventSkip` | Diff empty or changed |
| `EventDecision` | Model responded |
| `EventDelay` | About to sleep before retry |
| `EventCommit` | Commit succeeded |
| `EventForced` | Max delays hit, forcing commit |
| `EventError` | Git or AI error |
