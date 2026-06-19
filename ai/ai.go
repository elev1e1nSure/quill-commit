package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Decision struct {
	Commit  bool   `json:"commit"`
	Delay   int    `json:"delay"`
	Message string `json:"message"`
}

const (
	dialTimeout  = 10 * time.Second
	readTimeout  = 30 * time.Second
	openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
)

func Ask(diff, model, apiKey string) (Decision, error) {
	if diff == "" {
		return Decision{Commit: false, Delay: 1}, nil
	}

	prompt := `You are an automatic git committer.
You receive a git diff. Decide if a logical unit of work is complete.
Return ONLY json without markdown:
{"commit": bool, "delay": int (minutes if commit false), "message": string (if commit true)}

If commit is true, the message MUST follow Conventional Commits format:
  type(scope): short description
Rules:
- type is one of: feat, fix, refactor, perf, test, docs, chore, style, ci, build
- scope is a short lowercase word (package or area changed), optional but preferred
- description is imperative, lowercase, no period, max 72 characters total for the whole message
- entire message must be under 100 characters
Example: fix(ai): trim whitespace from model response`

	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": prompt},
			{"role": "user", "content": diff},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fallback(), fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fallback(), fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{
		Timeout: dialTimeout + readTimeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return Decision{Commit: false, Delay: 1}, err
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
		return fallback(), fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return fallback(), fmt.Errorf("empty response")
	}

	content := strings.Trim(result.Choices[0].Message.Content, " \n")
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var decision Decision
	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return fallback(), fmt.Errorf("parse decision: %w", err)
	}

	return decision, nil
}

func fallback() Decision {
	return Decision{Commit: true, Message: "auto: fallback commit"}
}
