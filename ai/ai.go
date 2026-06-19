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
	Commit  bool   `json:"commit"`
	Delay   int    `json:"delay"`
	Message string `json:"message"`
}

const (
	dialTimeout   = 10 * time.Second
	readTimeout   = 30 * time.Second
	openRouterURL = "https://openrouter.ai/api/v1/chat/completions"
)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	httpCli                 httpClient = &http.Client{Timeout: dialTimeout + readTimeout}
	openRouterModelsURL                = "https://openrouter.ai/api/v1/models"
	cacheCapabilityTimeout            = dialTimeout
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

	resp, err := httpCli.Do(req)
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
