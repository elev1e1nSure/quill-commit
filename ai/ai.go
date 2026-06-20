package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Decision struct {
	Commit  bool          `json:"commit"`
	Delay   int           `json:"delay"`
	Reason  string        `json:"reason"`
	Message string        `json:"message"`
	Commits []CommitGroup `json:"commits"`
}

type CommitGroup struct {
	Files   []string `json:"files"`
	Message string   `json:"message"`
}

const (
	dialTimeout = 10 * time.Second
	readTimeout = 30 * time.Second
)

const AmendBasePrompt = `You are amending a git commit. Given the original commit message and the additional diff being added to it, write a single updated commit message covering all changes.
Return ONLY json without markdown: {"commit": true, "delay": 0, "message": "type(scope): description"}`

const AmendRewritePrompt = `You are improving a git commit message. Given the original commit message, rewrite it to be clearer, more descriptive, and follow Conventional Commits format.
Return ONLY json without markdown: {"commit": true, "delay": 0, "message": "type(scope): description"}`

const promptPreamble = `You are an automatic git committer. You receive a filtered git diff and decide what to commit.

Files are pre-filtered: secrets, credentials, and .quillignore patterns are already excluded before you see the diff.

HARD RULES (apply in every strategy — never override these):
- NEVER commit binary files. If the diff shows "Binary files a/... and b/... differ" for any file, that file must be excluded.
- NEVER commit compiled build artifacts: executables, object files (.o, .a, .lib, .exe, .dll, .so, .dylib), generated protobuf files unless already tracked, lock file changes with no dependency update.
- Files NOT assigned to any commit group in a split are excluded from staging — use this to drop junk files.

Return ONLY json without markdown.
When committing: {"commit": true, "message": "type(scope): description"}
When waiting:    {"commit": false, "delay": 60, "reason": "one sentence — what is missing or wrong"}
For split:       {"commit": true, "commits": [{"files": ["path/a.go"], "message": "type(scope): description"}, ...]}

Use exact file paths from the diff headers (the path after "diff --git a/").

PRIOR DECISIONS block: if the user message begins with "PRIOR DECISIONS:", those are your own past conclusions about this exact diff. Read them before deciding. Do not reverse a prior "wait" decision unless the diff has visibly changed or enough time has passed to justify it. If you do change your decision, your "reason" must explain why.

SOLO vs SPLIT:
- SOLO: changes belong to the same task, feature, or bugfix. Code + its tests + its docs = always SOLO.
- SPLIT: changes are completely independent, different scopes, can each be reverted alone without breaking anything.
- When in doubt, default to SOLO.`

// PromptForStrategy returns the system prompt for the given commit strategy.
// Valid strategies: "permissive", "standard", "strict". Empty string or unknown → "standard".
func PromptForStrategy(strategy string) string {
	var block string
	switch strategy {
	case "permissive":
		block = `STRATEGY: permissive
Commit any non-binary, non-artifact change that reaches you.
- Set commit: true for any meaningful diff. Do not delay for code quality reasons.
- Debug prints, TODO comments, temp variables — include them. That is the developer's choice.
- Split only when changes are clearly unrelated across different scopes (e.g. a bug fix in one package and unrelated config edits in another).`
	case "strict":
		block = `STRATEGY: strict
Commit only clean, atomic, purposeful changes. Be demanding.
- Delay (commit: false, delay: 120) if: the diff mixes unrelated concerns, contains debug print statements added in this diff, adds TODO/FIXME markers for unfinished work, has incomplete implementations (half-written functions, removed code with no replacement), or the purpose of the change is not clear from the diff alone.
- Split aggressively: each commit must be independently meaningful and revertable without breaking the build.
- Reject messy diffs — the developer should clean up before committing.
- Commit messages must be precise. Reject vague messages like "update X" or "fix stuff" — delay instead until the change is unambiguous.`
	default: // "standard"
		block = `STRATEGY: standard
Commit complete, reasonable units of work. Use common sense.
- Delay (commit: false, delay: 60) if the diff looks incomplete: unclosed functions, obvious WIP markers ("// WIP", "// TODO: implement this"), half-removed code with no replacement.
- Skip obvious debug noise added in this diff: console.log spam, fmt.Println debugging, scratch variables named "tmp2", "xxx", "debug".
- DO commit: code changes together with their tests and documentation, config changes, complete features or bug fixes.
- Split when scopes are clearly independent.`
	}
	return promptPreamble + "\n\n" + block
}

// BasePrompt is the standard-strategy prompt, used when no strategy is configured.
var BasePrompt = PromptForStrategy("standard")

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	httpCli                httpClient = &http.Client{Timeout: dialTimeout + readTimeout}
	openRouterURL                     = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModelsURL               = "https://openrouter.ai/api/v1/models"
	cacheCapabilityTimeout            = dialTimeout
	CacheCapabilityFn                 = CacheCapability
)

type Request struct {
	SystemPrompt  string
	UserPrompt    string
	Model         string
	APIKey        string
	SessionID     string
	ExplicitCache bool
	Ctx           context.Context
}

type Usage struct {
	CachedTokens int
	PromptTokens int
}

func Ask(req Request) (Decision, Usage, error) {
	if req.UserPrompt == "" {
		return Decision{Commit: false, Delay: 30}, Usage{}, nil
	}

	var systemMsg any
	if !req.ExplicitCache {
		systemMsg = map[string]string{
			"role":    "system",
			"content": req.SystemPrompt,
		}
	} else {
		var content any
		if parts := strings.SplitN(req.SystemPrompt, "\n\n---\n\n", 2); len(parts) == 2 {
			content = []any{
				map[string]any{
					"type": "text",
					"text": parts[0],
				},
				map[string]any{
					"type":          "text",
					"text":          parts[1],
					"cache_control": map[string]string{"type": "ephemeral"},
				},
			}
		} else {
			content = []any{
				map[string]any{
					"type":          "text",
					"text":          req.SystemPrompt,
					"cache_control": map[string]string{"type": "ephemeral"},
				},
			}
		}
		systemMsg = map[string]any{
			"role":    "system",
			"content": content,
		}
	}

	messages := []any{
		systemMsg,
		map[string]string{
			"role":    "user",
			"content": req.UserPrompt,
		},
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}

	if req.SessionID != "" {
		body["session_id"] = req.SessionID
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fallback(), Usage{}, fmt.Errorf("marshal request: %w", err)
	}

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fallback(), Usage{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return Decision{Commit: false, Delay: 30}, Usage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fallback(), Usage{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fallback(), Usage{}, fmt.Errorf("decode response: %w", err)
	}

	var usage Usage
	usage.PromptTokens = result.Usage.PromptTokens
	if result.Usage.PromptTokensDetails != nil {
		usage.CachedTokens = result.Usage.PromptTokensDetails.CachedTokens
	}

	if len(result.Choices) == 0 {
		return fallback(), usage, fmt.Errorf("empty response")
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}
	var decision Decision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return fallback(), usage, fmt.Errorf("parse decision: %w", err)
	}

	return decision, usage, nil
}

func CacheCapability(model, apiKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cacheCapabilityTimeout)
	defer cancel()

	url := openRouterModelsURL + "/" + model
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpCli.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Data *struct {
			SupportedParameters *[]string `json:"supported_parameters"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}

	if result.Data == nil || result.Data.SupportedParameters == nil {
		return false, nil
	}

	for _, param := range *result.Data.SupportedParameters {
		if param == "cache_control" {
			return true, nil
		}
	}

	return false, nil
}

func fallback() Decision {
	return Decision{Commit: true, Message: "auto: fallback commit"}
}

const ExplainErrorPrompt = `You are a developer assistant explaining a git pre-commit hook failure.
Given the error output from a failed commit, provide a brief explanation and an actionable suggestion.
Return ONLY json without markdown: {"summary": "one-line description of what failed", "fix": "short actionable suggestion"}`

type Explanation struct {
	Summary string `json:"summary"`
	Fix     string `json:"fix"`
}

func AskExplain(req Request) (Explanation, error) {
	messages := []any{
		map[string]string{"role": "system", "content": ExplainErrorPrompt},
		map[string]string{"role": "user", "content": req.UserPrompt},
	}
	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return Explanation{}, fmt.Errorf("marshal: %w", err)
	}
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		return Explanation{}, fmt.Errorf("request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return Explanation{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Explanation{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Explanation{}, fmt.Errorf("decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return Explanation{}, fmt.Errorf("empty response")
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var expl Explanation
	if err := json.Unmarshal([]byte(content), &expl); err != nil {
		return Explanation{Summary: "commit hook failed", Fix: "check the full error with ctrl+o"}, nil
	}
	return expl, nil
}
