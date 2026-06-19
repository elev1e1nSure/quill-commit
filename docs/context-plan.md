# Context Implementation Plan

## Decisions

| # | Decision |
|---|----------|
| 1 | Static: `CLAUDE.md` > `README.md` > `AGENTS.md` from repo root |
| 2 | `recent_commits` in toml, default 10 |
| 3 | `context_budget` in chars (~tokens = chars/4) |
| 4 | 3 cache misses → shrink static to ~800 chars, reset on hit |
| 5 | `session_id` via `crypto/rand` per-run, optional override in toml |
| 6 | Query `/api/v1/models/{model}` at startup for `cache_control` |
| 7 | Packages from `git ls-files` (tracked), sorted |
| 8 | Commit conventions hardcoded in `context/` |
| 9 | `BuildDynamic` fail → skip dynamic, send static+diff |
| 10 | Cache hit/miss → stderr only, TUI untouched |

## Architecture

```
config/    + IncludeContext, ContextBudget, RecentCommitsCount, SessionID
git/       + RecentCommits, StatusShort, LsFiles, RepoRoot
context/   NEW — BuildStatic, BuildDynamic, RenderSystem, RenderUser, Hash
ai/        + CacheCapability; Ask(req Request) (Decision, Usage, err); BasePrompt
watcher/   collects static once, per-run session_id, cache-miss state machine
```

Request: system = `BasePrompt + "\n\n---\n\n" + RenderSystem(static)`, user = `RenderUser(dynamic, diff)`, top-level `session_id`. `cache_control` on static block iff `ExplicitCache`.

Stages 4+5 land together (Ask signature change breaks watcher mid-way).

## Stage 1 — git/

Add to `git/git.go` next to existing helpers, same pattern (`exec.Command` + `CombinedOutput` + `fmt.Errorf("git <sub>: %w", err)` + `TrimSpace`):

```go
func RecentCommits(n int) (string, error)  // git log --oneline -n N; n<=0 -> ("", nil); unborn branch (stderr "does not have any commits") -> ("", nil)
func StatusShort() (string, error)         // git status --short; clean -> ("", nil)
func LsFiles() (string, error)             // git ls-files; split, SORT lexically, rejoin
```

Tests: `n<=0`, unborn branch, 3 commits asking 3/10, clean/dirty tree, ls-files with subdirs. Reuse temp-repo fixture from `git_test.go`.

Verify: `just lint && just test`. Commit: `feat(git): add RecentCommits, StatusShort, LsFiles`.

Sonnet: `Open git/git.go and git_test.go. Add RecentCommits(n int) (git log --oneline -n N; n<=0 returns ("",nil); unborn branch detected by stderr "does not have any commits" returns ("",nil) not error), StatusShort (git status --short, clean -> ("",nil)), LsFiles (git ls-files, split on \n, sort slice, rejoin). Follow Diff() pattern exactly. Tests: n<=0, unborn, 3 commits asking 3 and 10, clean/dirty tree, ls-files subdirs. Reuse existing temp-repo fixture. just lint && just test. Do not commit.`

## Stage 1.5 — ai/ cache capability

```go
func CacheCapability(model, apiKey string) (bool, error)  // GET /api/v1/models/{model}, 10s timeout, parse supported_parameters []string, true if contains "cache_control"; missing field -> (false, nil); network/parse error -> (false, err)
```

Make HTTP client injectable: `var httpCli httpClient = &http.Client{Timeout: dialTimeout+readTimeout}` where `type httpClient interface{ Do(*http.Request) (*http.Response, error) }`. Use it in `Ask` too.

Tests via `httptest`: cache_control present/absent, field missing, 500, timeout. No network.

Verify: `just lint && just test`. Commit: `feat(ai): query model cache capability at startup`.

Sonnet: `In ai/ai.go add CacheCapability(model, apiKey string) (bool, error): GET https://openrouter.ai/api/v1/models/<model> with Bearer auth, 10s timeout (reuse dialTimeout). Parse supported_parameters []string; true if contains "cache_control"; missing field -> (false,nil); network/non-2xx/decode error -> (false,err). Make client injectable: package-private httpClient interface + var httpCli = &http.Client{Timeout: dialTimeout+readTimeout}, used by Ask too. Tests in ai_test.go via httptest: present, absent, missing field, 500, timeout (50ms timeout + slow handler). No network. just lint && just test. Do not commit.`

## Stage 2 — context/

```go
type Static struct { Project, Stack string; Packages []string; Conventions string }
type Dynamic struct { RecentCommits, ChangedFiles string }

func BuildStatic(repoRoot string) (Static, error)       // CLAUDE>README>AGENTS, parse ^## Project$ / ^## Stack$ (lines until next ^## ), Packages from git.LsFiles top-level dirs sorted
func BuildDynamic(commitsN int) (Dynamic, error)        // git.RecentCommits + git.StatusShort; partial result + err on failure
func RenderSystem(s Static, budgetChars int) string     // deterministic order: Project, Stack, Packages, Conventions; budget<=0 = no limit; over budget: drop Conventions, then Packages, then trim Stack, never Project
func RenderUser(d Dynamic, diff string) string          // only non-empty sections: Recent commits / Changed files / Diff
func (s Static) Hash() string                           // sha256 hex over RenderSystem with huge budget
```

`Conventions` hardcoded (move from ai/ai.go:29-41): conventional commits types + scope/description rules + example.

Package vars for tests: `var lsFilesFunc = git.LsFiles`, `var recentCommitsFunc = git.RecentCommits`, `var statusShortFunc = git.StatusShort`.

Tests: CLAUDE with both sections, README fallback, no doc file (empty fields, no err), missing Stack, BuildDynamic happy + RecentCommits fail, RenderSystem deterministic, budget cap honored, truncation order (Conventions first), RenderUser omits empty, Hash stable, Hash reflects sorted Packages.

Verify: `just lint && just test`. Commit: `feat(context): add BuildStatic/BuildDynamic with prompt rendering`.

Sonnet: `New package context/ (context.go + context_test.go). API: Static{Project,Stack string; Packages []string; Conventions string}, Dynamic{RecentCommits,ChangedFiles string}, BuildStatic(repoRoot string)(Static,error), BuildDynamic(commitsN int)(Dynamic,error), RenderSystem(s Static, budgetChars int) string, RenderUser(d Dynamic, diff string) string, (s Static) Hash() string. BuildStatic picks CLAUDE.md>README.md>AGENTS.md, parses ^## Project$ and ^## Stack$ (body = lines until next ^## , trimmed; missing = empty, not error); Packages from git.LsFiles top-level segments (before first /), dedupe, sort. Conventions hardcoded string with conventional commits rules (types feat/fix/refactor/perf/test/docs/chore/style/ci/build, scope lowercase optional, desc imperative lowercase no period max 72, total <100, example fix(ai): trim whitespace). BuildDynamic calls git.RecentCommits + git.StatusShort; on either error returns partial Dynamic + err. RenderSystem deterministic order Project/Stack/Packages/Conventions with labels; budgetChars>0 -> output <= budget, drop Conventions first then Packages then trim Stack, never Project; <=0 no limit. RenderUser omits empty sections. Hash = sha256 hex of RenderSystem with 1<<20 budget. Package vars lsFilesFunc/recentCommitsFunc/statusShortFunc defaulting to git.* for test injection. Tests: CLAUDE both sections, README fallback, no doc, missing Stack, BuildDynamic happy + fail, RenderSystem deterministic + budget cap + truncation order, RenderUser empty sections omitted, Hash stable + sort-sensitive. No new deps. just lint && just test. Do not commit.`

## Stage 3 — config/

Add fields to `Config`: `IncludeContext bool` (default true), `ContextBudget int` (default 8000), `RecentCommitsCount int` (default 10), `SessionID string` (default "").

Constants: `DefaultIncludeContext=true`, `DefaultContextBudget=8000`, `DefaultRecentCommitsCount=10`.

`Default()` sets them. `Load()` guards: `ContextBudget<=0 → default`, `RecentCommitsCount<=0 → default`. `IncludeContext=false` is valid user choice, don't override. `SessionID=""` is valid.

Tests: Default() values, Load with `include_context=false` stays false, missing fields apply defaults, `context_budget=0` → 8000, `recent_commits=-1` → 10, round-trip Save→Load.

Verify: `just lint && just test`. Commit: `feat(config): add include_context, context_budget, recent_commits, session_id`.

Sonnet: `Extend config.Config with IncludeContext bool, ContextBudget int, RecentCommitsCount int, SessionID string (toml tags include_context/context_budget/recent_commits/session_id). Constants DefaultIncludeContext=true, DefaultContextBudget=8000, DefaultRecentCommitsCount=10. Update Default() to set them. Load() zero-guards: ContextBudget<=0 -> default, RecentCommitsCount<=0 -> default; IncludeContext=false is valid (don't override); SessionID="" is valid. Tests: Default() values, Load with include_context=false stays false, missing fields apply defaults, context_budget=0 -> 8000, recent_commits=-1 -> 10, Save->Load round-trip. Follow config_test.go style. just lint && just test. Do not commit.`

## Stage 4 — ai/ Ask refactor

```go
type Request struct { SystemPrompt, UserPrompt, Model, APIKey, SessionID string; ExplicitCache bool }
type Usage struct { CachedTokens, PromptTokens int }
func Ask(req Request) (Decision, Usage, error)
```

`BasePrompt` const in ai.go (role + JSON shape only, WITHOUT commit rules — those come from context). Caller sends `BasePrompt + "\n\n---\n\n" + RenderSystem(...)`.

Body:
- top-level `session_id` iff `SessionID != ""`
- `ExplicitCache=false` → system as string
- `ExplicitCache=true` → system as content-block array; split `SystemPrompt` on first `"\n\n---\n\n"`: first block (base) uncached, second (static) with `cache_control:{type:ephemeral}`. If no separator, single cached block.
- user as string

Decode `usage.prompt_tokens` → `Usage.PromptTokens`, `usage.prompt_tokens_details.cached_tokens` → `Usage.CachedTokens`. Absent = 0. Keep fallback paths.

Breaks watcher compilation — ok, Stage 5 fixes. Verify with `go test ./ai/... ./config/... ./context/... ./git/...` (not `just test`). Commit: `refactor(ai): switch Ask to Request, send session_id and cache_control`.

Sonnet: `Refactor ai/ai.go: replace Ask(diff,model,apiKey) with Request{SystemPrompt,UserPrompt,Model,APIKey,SessionID string; ExplicitCache bool} and Ask(req Request)(Decision, Usage{CachedTokens,PromptTokens int}, error). Add const BasePrompt = role + JSON shape only (no commit rules, those come from context). Body: top-level session_id iff non-empty; ExplicitCache=false -> system as string; ExplicitCache=true -> system as content-block array, split SystemPrompt on first "\n\n---\n\n" -> first block uncached, second block with cache_control:{type:ephemeral}, no separator -> single cached block; user as string. Decode usage.prompt_tokens and usage.prompt_tokens_details.cached_tokens (absent=0). Keep existing decision parsing and fallback behavior. Breaks watcher.aiOps intentionally (Stage 5 fixes). Update ai_test.go: ExplicitCache true/false body shape, SessionID present/absent, cached_tokens parsed, missing prompt_tokens_details -> 0, fallback paths still work. Use httptest. Verify go test ./ai/... ./config/... ./context/... ./git/... (not just test, watcher won't compile). Do not commit.`

## Stage 5 — watcher/ integration

`Watcher` gets: `static context.Static`, `staticBudget int`, `fullBudget int`, `sessionID string`, `explicitCache bool`, `cacheMisses int`.

`New(cfg, apiKey, repoRoot string)`:
- `cfg.IncludeContext`: `context.BuildStatic(repoRoot)` (err → warn stderr, empty Static); sessionID from `cfg.SessionID` or `crypto/rand` 16 bytes hex; `ai.CacheCapability` (err → warn, false); `staticBudget=fullBudget=cfg.ContextBudget`.
- else: all zero, `sessionID=""`.

Add `git.RepoRoot()` (`git rev-parse --show-toplevel`) + test.

`aiOps` interface: `Ask(req ai.Request) (ai.Decision, ai.Usage, error)`. Update `realAI`.

`delayLoop` before each Ask:
1. `dyn, dynErr := context.BuildDynamic(cfg.RecentCommitsCount)` — err → warn stderr, proceed with partial.
2. `sysPrompt`: `IncludeContext` ? `ai.BasePrompt + "\n\n---\n\n" + context.RenderSystem(static, staticBudget)` : `ai.BasePrompt`.
3. `userPrompt`: `IncludeContext` ? `context.RenderUser(dyn, stableDiff)` : `stableDiff`.
4. `ai.Ask(Request{...})`.
5. `usage.CachedTokens > 0` → if `cacheMisses>0 || staticBudget<fullBudget` reset both; `fmt.Fprintf(os.Stderr, "cache: hit %d tok\n", ...)`. Else `cacheMisses++`; `fmt.Fprintf(os.Stderr, "cache: miss (%d)\n", cacheMisses)`; if `cacheMisses>=3 && staticBudget>800` → `staticBudget=800`, `cacheMisses=0`, log shrink.

`main.go`: `repoRoot, err := git.RepoRoot()` (err → exit 1), pass to `watcher.New`.

Tests: happy path asserts Request has `SessionID!=""`, `ExplicitCache` matches injected `CacheCapability` (via package var `ai.CacheCapabilityFn`), SystemPrompt contains static. Three misses → 4th Request shorter, 5th hit → 6th restores full. `IncludeContext=false` → `SessionID==""`, `UserPrompt==diff`. `BuildDynamic` fail (inject `context.recentCommitsFunc`) → no crash, Ask still called. `BuildStatic` fail (inject `context.lsFilesFunc`) → `New` no panic, Request.SystemPrompt == BasePrompt only.

Verify: `just lint && just test`. Commit: `feat(watcher): inject project context and session_id into AI requests`.

Sonnet: `Open watcher/watcher.go, watcher_test.go, main.go, ai/ai.go, git/git.go. (1) Add const BasePrompt in ai/ai.go with role+JSON shape only. (2) Add git.RepoRoot() = git rev-parse --show-toplevel trimmed; test in subdir of temp repo. (3) Update watcher.aiOps to Ask(req ai.Request)(ai.Decision, ai.Usage, error); update realAI. (4) Extend Watcher: static context.Static, staticBudget/fullBudget int, sessionID string, explicitCache bool, cacheMisses int. (5) New(cfg, apiKey, repoRoot string): if IncludeContext -> BuildStatic (err -> warn stderr + empty Static), sessionID from cfg.SessionID or crypto/rand 16 bytes hex, ai.CacheCapability (err -> warn + false), staticBudget=fullBudget=cfg.ContextBudget; else all zero, sessionID="". (6) delayLoop before each Ask: BuildDynamic (err -> warn + proceed), sysPrompt = IncludeContext ? BasePrompt+"\n\n---\n\n"+RenderSystem(static, staticBudget) : BasePrompt, userPrompt = IncludeContext ? RenderUser(dyn, stableDiff) : stableDiff, call Ask(Request{...}). Inspect usage.CachedTokens: >0 -> if cacheMisses>0||staticBudget<fullBudget reset both, log "cache: hit N tok" to stderr; ==0 -> cacheMisses++, log "cache: miss (N)", if cacheMisses>=3 && staticBudget>800 -> staticBudget=800, cacheMisses=0, log shrink. (7) main.go: git.RepoRoot() err -> exit 1, pass to watcher.New. (8) Update watcher_test.go mocks to new aiOps. Tests: happy path (Request.SessionID!="" , ExplicitCache matches injected CacheCapabilityFn, SystemPrompt contains static), three misses then shrink (4th Request shorter, 5th hit restores), IncludeContext=false (SessionID=="", UserPrompt==diff), BuildDynamic fail (no crash, Ask called), BuildStatic fail (New no panic, SystemPrompt == BasePrompt). Inject via package vars ai.CacheCapabilityFn, context.recentCommitsFunc, context.lsFilesFunc. just lint && just test. Do not commit.`

## Stage 6 — tests sweep

Fill gaps: `context.Hash` called twice identical; `RenderSystem` budget=1 no panic; `Ask` ExplicitCache=true no separator → single cached block; `watcher` cfg.SessionID="explicit-id" → Request.SessionID matches; `git.RepoRoot` from subdir; no test needs network or QUILL_API_KEY.

Verify: `just lint && just test` + `go test -race ./...`. Commit (if new tests): `test(*): cover context, ai Request, watcher cache fallback`.

Sonnet: `Fill test gaps: context.Hash twice identical, RenderSystem budget=1 no panic, ai.Ask ExplicitCache=true with no "---" separator -> single cached block, watcher cfg.SessionID="explicit-id" -> Request.SessionID matches, git.RepoRoot from subdir of temp repo, no test needs network/QUILL_API_KEY. Run just lint && just test and go test -race ./.... Do not commit.`

## Stage 7 — docs

`docs/architecture.md`: new section "Project context and prompt caching" after "Package layout" — static (CLAUDE>README>AGENTS, packages from ls-files sorted, hardcoded conventions) vs dynamic (recent commits, status), request shape, session_id sticky routing, CacheCapability probe, 3-miss shrink/recover state machine, failure modes (BuildStatic/BuildDynamic/CacheCapability → degrade gracefully). Update Package layout to add `context/`.

`docs/development.md`: mention `context/` and new `git/` helpers.

Verify: `just lint`. Commit: `docs(architecture): document project context and prompt caching`.

Sonnet: `Update docs/architecture.md: add "Project context and prompt caching" section after "Package layout" (static from CLAUDE>README>AGENTS + ls-files packages + hardcoded conventions; dynamic = recent commits + status; request shape system=BasePrompt+static user=dynamic+diff; session_id for sticky routing; CacheCapability /models probe; 3 cache misses -> shrink to 800 chars, hit -> restore; failure modes BuildStatic/BuildDynamic/CacheCapability degrade gracefully). Add context/ to Package layout. Update docs/development.md to mention context/ and new git helpers (RecentCommits, StatusShort, LsFiles, RepoRoot). just lint. Do not commit.`

## Risks

- **`cache_control` field name**: Stage 1.5 assumes `supported_parameters` contains literal `"cache_control"`. Verify with `curl https://openrouter.ai/api/v1/models/anthropic/claude-sonnet-4` before trusting. Fallback `false` is safe regardless.
- **DeepSeek min cache size**: if 8000 chars below threshold, `cacheMisses` triggers shrink after 3 requests; stderr logs reveal it. Tune `DefaultContextBudget` if needed.
- **Hash stability**: `LsFiles` sort + deterministic `RenderSystem` field order are the guards. Stage 6 has stability test.
- **Stages 4+5 atomic**: `just test` only passes after Stage 5. Stage 4 verify uses `go test ./ai/... ./config/... ./context/... ./git/...`.
- **`IncludeContext=false` tri-state**: toml bool zero is `false`, indistinguishable from explicit disable. `Default()` sets `true` so fresh config is on; user writing `include_context=false` gets off. Documented.
