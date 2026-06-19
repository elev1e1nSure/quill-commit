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
	dialTimeout = 10 * time.Second
	readTimeout = 30 * time.Second
)

const BasePrompt = `You are an automatic git committer.
You receive a git diff. Decide if a logical unit of work is complete.
Return ONLY json without markdown:
{"commit": bool, "delay": int (minutes if commit false), "message": string (if commit true)}`

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	httpCli                 httpClient = &http.Client{Timeout: dialTimeout + readTimeout}
	openRouterURL                      = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModelsURL                = "https://openrouter.ai/api/v1/models"
	cacheCapabilityTimeout            = dialTimeout
	CacheCapabilityFn                  = CacheCapability
)

type Request struct {
	SystemPrompt  string
	UserPrompt    string
	Model         string
	APIKey        string
	SessionID     string
	ExplicitCache bool
}

type Usage struct {
	CachedTokens int
	PromptTokens int
}

func Ask(req Request) (Decision, Usage, error) {
	if req.UserPrompt == "" {
		return Decision{Commit: false, Delay: 1}, Usage{}, nil
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

	httpReq, err := http.NewRequest(http.MethodPost, openRouterURL, bytes.NewReader(jsonBody))
	if err != nil {
		return fallback(), Usage{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)

	resp, err := httpCli.Do(httpReq)
	if err != nil {
		return Decision{Commit: false, Delay: 1}, Usage{}, err
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

	content := strings.Trim(result.Choices[0].Message.Content, " \n")
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
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
