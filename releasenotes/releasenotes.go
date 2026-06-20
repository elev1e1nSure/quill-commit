package releasenotes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

var openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

const (
	dialTimeout = 10 * time.Second
	readTimeout = 30 * time.Second
)

var client = &http.Client{Timeout: dialTimeout + readTimeout}

func Generate(ctx context.Context, fromRef, toRef, apiKey, model string, initial bool) (string, error) {
	if fromRef == toRef {
		return "", fmt.Errorf("from and to revisions are identical: %s", fromRef)
	}
	commits, err := getCommits(ctx, fromRef, toRef)
	if err != nil {
		return "", fmt.Errorf("get commits: %w", err)
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits between %s and %s", fromRef, toRef)
	}

	prompt := buildPrompt()
	if initial {
		prompt = buildInitialPrompt()
	}
	return chat(ctx, model, apiKey, prompt, strings.Join(commits, "\n"))
}

func getCommits(ctx context.Context, fromRef, toRef string) ([]string, error) {
	arg := fmt.Sprintf("%s..%s", fromRef, toRef)
	out, err := exec.CommandContext(ctx, "git", "log", "--pretty=format:%s", arg).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

func buildPrompt() string {
	return `You are a changelog editor. Convert raw commit messages into polished release notes that look hand-written.

Core principle: only include changes a user can notice, verify, or benefit from.

Rules:
- Keep the existing structure: ### section headers with emojis, then bullet list. You may rename section headers if they misrepresent the content.
- Rename '🔧 Miscellaneous Tasks' to '🔧 Maintenance'.
- Rewrite each commit into a clear user-facing description. Expand abbreviations, fix grammar, make it natural English.
- Summary line at the top: one sentence for patches, two sentences max for minor releases. Never use grandiose language ("major overhaul", "complete redesign", etc.).
- NEVER include any of these — they provide zero value to users:
  * Version bumps ("bump version to…", "chore: bump")
  * CI/CD config changes, workflow changes, release infrastructure
  * .gitignore changes
  * Agent/config files (CLAUDE.md, AGENTS.md, rules.md, etc.)
  * Dependency updates without a visible fix
  * Branch-only commits, merge commits, revert commits
  * Internal refactors with no visible user effect (e.g. "replaced switch-case with handler map")
  * Dead code removal, "cleanup model fields", "remove unused code"
  * Changelog/infra tooling (git-cliff config, AI beautifier script, etc.)
- Merge truly trivial adjacent commits into one bullet. Keep meaningful changes separate.
- Refactor commits: include ONLY if there's a visible result. Instead of "refactored X" write "X is now more reliable/faster/clearer".
- Do NOT invent features, fixes, or details not present in the commits.
- Return ONLY the final markdown. No preamble, no code fences.
- Never mention AI, LLM, machine generation, or the beautification process itself. The changelog must read as if written by a human.

COMMITS:`
}

func buildInitialPrompt() string {
	return `You are writing the first release notes for a new CLI tool. Describe what it does, not how it was built.

Rules:
- Write from the user's perspective: what can they do with this tool, what problems does it solve
- 3-5 bullets under ### ✨ Features, each describing a capability or behavior the user will notice
- No Fixes section — this is a first release
- Skip all implementation details: CI setup, refactors, test infrastructure, internal cleanups
- One short summary sentence at the top (not a header, just a sentence)
- Return ONLY the markdown. No preamble, no code fences, never mention that this is AI-generated

COMMITS:`
}

func chat(ctx context.Context, model, apiKey, systemPrompt, userContent string) (string, error) {
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty AI response (no choices)")
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```markdown")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), nil
}
