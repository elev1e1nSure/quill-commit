# quill-commit — Idea

## What it is

A background tool that sits in a git repo and auto-commits your work for you.
You write code, `quill-commit` watches the diffs, decides when a logical unit is done, and commits it with a meaningful message — no manual `git commit` needed.

## Why it exists

Developers lose commit discipline under pressure: either commit too rarely (one giant commit at the end) or too often (noise commits like "fix", "wip", "asdf"). Both make git history useless.

`quill-commit` takes the decision out of the loop. An LLM observes the actual diff and decides when the change is coherent enough to commit — like having a senior dev watching over your shoulder and saying "okay, that's a unit, commit it".

## How it works

1. Runs a ticker every N minutes, captures `git diff HEAD`
2. Waits for the diff to stabilize (same between two consecutive ticks = work paused)
3. Sends the stable diff to an LLM via OpenRouter
4. LLM decides: commit now, or wait a bit longer
5. On commit — `git add -A && git commit -m "<generated message>"`
6. On delay — waits the suggested time, retries
7. If too many delays in a row — force-commits anyway (`auto: forced commit`)

## Core design decisions

- **Stabilization over immediacy** — don't commit mid-edit; wait for the diff to stop changing between ticks
- **LLM as the only oracle** — no heuristics, no line-count thresholds; the model sees the actual diff and reasons about it
- **Force-commit as a safety net** — `max_delays` ensures nothing is ever lost, even if the model keeps saying "not yet"
- **Network errors don't count** — a failed API call is not a delay; the counter only increments on explicit model `commit: false`
- **Minimal TUI** — Status + Log; no clutter; verbose/diff view reserved for later

## What it is not

- Not a CI tool
- Not a code reviewer
- Not a replacement for deliberate commits on important branches
- Not opinionated about branching strategy
