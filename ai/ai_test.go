package ai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFallback(t *testing.T) {
	d := fallback()
	if !d.Commit {
		t.Error("fallback should set Commit=true")
	}
	if d.Message != "auto: fallback commit" {
		t.Errorf("fallback message mismatch: %s", d.Message)
	}
}

func TestAskEmptyDiff(t *testing.T) {
	d, usage, err := Ask(Request{
		UserPrompt: "",
		Model:      "model",
		APIKey:     "key",
	})
	if err != nil {
		t.Fatalf("Ask with empty diff failed: %v", err)
	}
	if d.Commit {
		t.Error("empty diff should not commit")
	}
	if d.Delay != 1 {
		t.Errorf("expected delay 1, got %d", d.Delay)
	}
	if usage.PromptTokens != 0 || usage.CachedTokens != 0 {
		t.Errorf("expected zero usage for empty diff, got %+v", usage)
	}
}

func TestAsk(t *testing.T) {
	type testResponse struct {
		StatusCode int
		Body       string
	}

	tests := []struct {
		name         string
		req          Request
		resp         testResponse
		verifyReq    func(t *testing.T, body map[string]any)
		expectedDec  Decision
		expectedUsg  Usage
		expectError  bool
	}{
		{
			name: "ExplicitCache=false and SessionID absent",
			req: Request{
				SystemPrompt:  "system-prompt",
				UserPrompt:    "user-prompt",
				Model:         "test-model",
				APIKey:        "test-key",
				SessionID:     "",
				ExplicitCache: false,
			},
			resp: testResponse{
				StatusCode: 200,
				Body:       `{"choices":[{"message":{"content":"{\"commit\":true,\"delay\":0,\"message\":\"feat: ok\"}"}}],"usage":{"prompt_tokens":10}}`,
			},
			verifyReq: func(t *testing.T, body map[string]any) {
				if _, ok := body["session_id"]; ok {
					t.Error("session_id should not be present in request")
				}
				messages, ok := body["messages"].([]any)
				if !ok || len(messages) != 2 {
					t.Fatalf("expected 2 messages, got %v", body["messages"])
				}
				systemMsg, ok := messages[0].(map[string]any)
				if !ok {
					t.Fatalf("expected systemMsg to be map[string]any, got %T", messages[0])
				}
				if systemMsg["role"] != "system" {
					t.Errorf("expected role system, got %v", systemMsg["role"])
				}
				if systemMsg["content"] != "system-prompt" {
					t.Errorf("expected content system-prompt, got %v", systemMsg["content"])
				}
			},
			expectedDec: Decision{Commit: true, Delay: 0, Message: "feat: ok"},
			expectedUsg: Usage{PromptTokens: 10, CachedTokens: 0},
			expectError: false,
		},
		{
			name: "ExplicitCache=true and SessionID present",
			req: Request{
				SystemPrompt:  "system-prompt",
				UserPrompt:    "user-prompt",
				Model:         "test-model",
				APIKey:        "test-key",
				SessionID:     "session-123",
				ExplicitCache: true,
			},
			resp: testResponse{
				StatusCode: 200,
				Body:       `{"choices":[{"message":{"content":"{\"commit\":false,\"delay\":5,\"message\":\"\"}"}}],"usage":{"prompt_tokens":100,"prompt_tokens_details":{"cached_tokens":40}}}`,
			},
			verifyReq: func(t *testing.T, body map[string]any) {
				if body["session_id"] != "session-123" {
					t.Errorf("expected session_id session-123, got %v", body["session_id"])
				}
				messages, ok := body["messages"].([]any)
				if !ok || len(messages) != 2 {
					t.Fatalf("expected 2 messages, got %v", body["messages"])
				}
				systemMsg, ok := messages[0].(map[string]any)
				if !ok {
					t.Fatalf("expected systemMsg to be map, got %T", messages[0])
				}
				content, ok := systemMsg["content"].([]any)
				if !ok || len(content) != 1 {
					t.Fatalf("expected content block array size 1, got %v", systemMsg["content"])
				}
				block, ok := content[0].(map[string]any)
				if !ok {
					t.Fatalf("expected block to be map, got %T", content[0])
				}
				if block["type"] != "text" || block["text"] != "system-prompt" {
					t.Errorf("unexpected content block: %v", block)
				}
				cacheCtrl, ok := block["cache_control"].(map[string]any)
				if !ok || cacheCtrl["type"] != "ephemeral" {
					t.Errorf("expected cache_control type ephemeral, got %v", block["cache_control"])
				}
			},
			expectedDec: Decision{Commit: false, Delay: 5, Message: ""},
			expectedUsg: Usage{PromptTokens: 100, CachedTokens: 40},
			expectError: false,
		},
		{
			name: "ExplicitCache=true with prompt splitting",
			req: Request{
				SystemPrompt:  "block1\n\n---\n\nblock2",
				UserPrompt:    "user-prompt",
				Model:         "test-model",
				APIKey:        "test-key",
				SessionID:     "",
				ExplicitCache: true,
			},
			resp: testResponse{
				StatusCode: 200,
				Body:       `{"choices":[{"message":{"content":"{\"commit\":true,\"delay\":0,\"message\":\"refactor: clean\"}"}}],"usage":{"prompt_tokens":50,"prompt_tokens_details":{"cached_tokens":25}}}`,
			},
			verifyReq: func(t *testing.T, body map[string]any) {
				messages, ok := body["messages"].([]any)
				if !ok || len(messages) != 2 {
					t.Fatalf("expected 2 messages, got %v", body["messages"])
				}
				systemMsg, ok := messages[0].(map[string]any)
				if !ok {
					t.Fatalf("expected systemMsg to be map, got %T", messages[0])
				}
				content, ok := systemMsg["content"].([]any)
				if !ok || len(content) != 2 {
					t.Fatalf("expected content block array size 2, got %v", systemMsg["content"])
				}
				block1, ok1 := content[0].(map[string]any)
				if !ok1 {
					t.Fatalf("expected block1 to be map, got %T", content[0])
				}
				block2, ok2 := content[1].(map[string]any)
				if !ok2 {
					t.Fatalf("expected block2 to be map, got %T", content[1])
				}
				if block1["type"] != "text" || block1["text"] != "block1" || block1["cache_control"] != nil {
					t.Errorf("unexpected block1: %v", block1)
				}
				if block2["type"] != "text" || block2["text"] != "block2" {
					t.Errorf("unexpected block2 text: %v", block2)
				}
				cacheCtrl, ok := block2["cache_control"].(map[string]any)
				if !ok || cacheCtrl["type"] != "ephemeral" {
					t.Errorf("expected block2 cache_control type ephemeral, got %v", block2["cache_control"])
				}
			},
			expectedDec: Decision{Commit: true, Delay: 0, Message: "refactor: clean"},
			expectedUsg: Usage{PromptTokens: 50, CachedTokens: 25},
			expectError: false,
		},
		{
			name: "Status 500 error triggers fallback",
			req: Request{
				SystemPrompt: "system-prompt",
				UserPrompt:   "user-prompt",
				APIKey:       "key",
			},
			resp: testResponse{
				StatusCode: 500,
				Body:       "Internal Server Error",
			},
			expectedDec: Decision{Commit: true, Message: "auto: fallback commit"},
			expectedUsg: Usage{PromptTokens: 0, CachedTokens: 0},
			expectError: true,
		},
		{
			name: "JSON decode error on bad response format triggers fallback",
			req: Request{
				SystemPrompt: "system-prompt",
				UserPrompt:   "user-prompt",
				APIKey:       "key",
			},
			resp: testResponse{
				StatusCode: 200,
				Body:       `{"choices":[],"usage":{"prompt_tokens":12}}`,
			},
			expectedDec: Decision{Commit: true, Message: "auto: fallback commit"},
			expectedUsg: Usage{PromptTokens: 12, CachedTokens: 0},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+tt.req.APIKey {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				var reqBody map[string]any
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if tt.verifyReq != nil {
					tt.verifyReq(t, reqBody)
				}
				w.WriteHeader(tt.resp.StatusCode)
				if _, err := w.Write([]byte(tt.resp.Body)); err != nil {
					t.Errorf("write error: %v", err)
				}
			}))
			defer server.Close()

			oldURL := openRouterURL
			oldCli := httpCli

			openRouterURL = server.URL
			httpCli = server.Client()

			defer func() {
				openRouterURL = oldURL
				httpCli = oldCli
			}()

			dec, usg, err := Ask(tt.req)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
			if dec.Commit != tt.expectedDec.Commit || dec.Delay != tt.expectedDec.Delay || dec.Message != tt.expectedDec.Message {
				t.Errorf("expected decision %+v, got %+v", tt.expectedDec, dec)
			}
			if usg.PromptTokens != tt.expectedUsg.PromptTokens || usg.CachedTokens != tt.expectedUsg.CachedTokens {
				t.Errorf("expected usage %+v, got %+v", tt.expectedUsg, usg)
			}
		})
	}
}

func TestCacheCapability(t *testing.T) {
	tests := []struct {
		name            string
		handler         http.HandlerFunc
		overrideTimeout time.Duration
		model           string
		apiKey          string
		expectedBool    bool
		expectError     bool
	}{
		{
			name: "present",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-key" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/test-model") {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"data":{"supported_parameters":["tools","cache_control"]}}`)); err != nil {
					t.Errorf("write error: %v", err)
				}
			},
			model:        "test-model",
			apiKey:       "test-key",
			expectedBool: true,
			expectError:  false,
		},
		{
			name: "absent",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"data":{"supported_parameters":["tools","something_else"]}}`)); err != nil {
					t.Errorf("write error: %v", err)
				}
			},
			model:        "test-model",
			apiKey:       "test-key",
			expectedBool: false,
			expectError:  false,
		},
		{
			name: "missing field - empty object",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{}`)); err != nil {
					t.Errorf("write error: %v", err)
				}
			},
			model:        "test-model",
			apiKey:       "test-key",
			expectedBool: false,
			expectError:  false,
		},
		{
			name: "missing field - data null",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"data":null}`)); err != nil {
					t.Errorf("write error: %v", err)
				}
			},
			model:        "test-model",
			apiKey:       "test-key",
			expectedBool: false,
			expectError:  false,
		},
		{
			name: "missing field - supported_parameters null",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(`{"data":{"supported_parameters":null}}`)); err != nil {
					t.Errorf("write error: %v", err)
				}
			},
			model:        "test-model",
			apiKey:       "test-key",
			expectedBool: false,
			expectError:  false,
		},
		{
			name: "500 error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			model:        "test-model",
			apiKey:       "test-key",
			expectedBool: false,
			expectError:  true,
		},
		{
			name: "timeout case",
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
			overrideTimeout: 50 * time.Millisecond,
			model:           "test-model",
			apiKey:          "test-key",
			expectedBool:    false,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			// Back up package globals
			openRouterModelsURLVal := openRouterModelsURL
			oldCli := httpCli
			oldTimeout := cacheCapabilityTimeout

			// Apply overrides
			openRouterModelsURL = server.URL
			httpCli = server.Client()
			if tt.overrideTimeout > 0 {
				cacheCapabilityTimeout = tt.overrideTimeout
			}

			// Defer restore
			defer func() {
				openRouterModelsURL = openRouterModelsURLVal
				httpCli = oldCli
				cacheCapabilityTimeout = oldTimeout
			}()

			res, err := CacheCapability(tt.model, tt.apiKey)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if res != tt.expectedBool {
					t.Errorf("expected %v, got %v", tt.expectedBool, res)
				}
			}
		})
	}
}
