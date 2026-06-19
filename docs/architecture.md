# Architecture

## Why it exists

Commit discipline degrades under pressure. Developers either commit too rarely (one giant dump at the end) or too often (noise: "fix", "wip", "asdf"). Both make history useless.

`quill-commit` takes the decision out of the loop. The LLM sees the actual diff and decides when a change is coherent — like a senior dev saying "okay, that's a unit, commit it."

## Watcher logic

```
tick (every interval)
 ├─ diff empty?   → skip, wait for next tick
 └─ diff changed? → stabilization loop (sleep stabilize, re-check)
      └─ diff stable (same two checks in a row)? → send to model
              ├─ commit: true  → git add -A && git commit
              └─ commit: false → sleep delay, retry model (not next tick)
                                  └─ max_delays hit → force commit
```

After any commit, delay counter and stored diff both reset.

## Key design decisions

**Two-speed stabilization.** `interval` controls how often the watcher starts a fresh check when nothing is happening. Once a non-empty diff appears, the watcher switches to `stabilize` re-checks (typically `interval / 2`) until the diff is unchanged — only then does it send to the model. This matters most for the `aggressive` preset: a 30s interval with a 15s stabilize re-check catches the pause-between-bursts faster than waiting another 30s for the next ticker.

**LLM as the only oracle.** No heuristics, no line-count thresholds. The model sees the full diff — tracked changes plus untracked files — and reasons about logical completeness. Invalid JSON from the model → fallback commit (`auto: fallback commit`).

**Network errors don't count.** A failed API call is not a delay. The counter only increments on explicit `commit: false`. This prevents network flakiness from burning through `max_delays`.

**Force-commit as a safety net.** `max_delays` ensures nothing is ever silently lost, even if the model keeps saying "not yet."

**Delay loop vs. ticker.** When the model says wait, the watcher sleeps inline (not waiting for the next tick). This decouples model-suggested delays from the stabilization interval — a 5-minute delay doesn't have to align with a 10-minute tick.

## Package layout

```
config/    Config struct, quill.toml read/write
git/       Diff, Add, Commit, IsRepo, HeadHash
ai/        OpenRouter request, Decision struct, fallback
watcher/   Ticker, stabilization, delay loop, event emission
ui/        Bubbletea TUI — Status block + Log block
main.go    Flag parsing, startup checks, wires everything together
```

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
