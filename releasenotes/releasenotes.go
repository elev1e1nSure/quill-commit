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

func Generate(ctx context.Context, fromRef, toRef, apiKey, model string) (string, error) {
	commits, err := getCommits(fromRef, toRef)
	if err != nil {
		return "", fmt.Errorf("get commits: %w", err)
	}
	if len(commits) == 0 {
		return "", fmt.Errorf("no commits between %s and %s", fromRef, toRef)
	}

	return chat(ctx, model, apiKey, buildPrompt(), strings.Join(commits, "\n"))
}

func getCommits(fromRef, toRef string) ([]string, error) {
	arg := fmt.Sprintf("%s..%s", fromRef, toRef)
	out, err := exec.Command("git", "log", arg, "--pretty=format:%s").CombinedOutput()
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
	return `You are a release notes editor. Transform these conventional commits into user-friendly release notes in English.

RULES:
- Categorize by type with emoji headers, sorted by importance:
  ✨ Features  🐛 Fixes  ⚡ Performance  ♻️ Refactoring  📝 Docs
- Drop ALL commits of type: chore, ci, style, test, build — they are internal noise with zero user value
- Drop commits that describe only implementation details with no user-facing impact
- Rewrite technical commit messages into plain, user-friendly language
- One bullet per entry, concise and human-readable
- Omit any category that has zero entries
- Return ONLY the markdown content, no preamble or explanation

FORMAT:
## ✨ Features
- User-friendly feature description

## 🐛 Fixes
- User-friendly fix description

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

	client := &http.Client{Timeout: dialTimeout + readTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

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
