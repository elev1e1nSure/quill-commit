package ai

import (
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
	d, err := Ask("", "model", "key")
	if err != nil {
		t.Fatalf("Ask with empty diff failed: %v", err)
	}
	if d.Commit {
		t.Error("empty diff should not commit")
	}
	if d.Delay != 1 {
		t.Errorf("expected delay 1, got %d", d.Delay)
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
			oldURL := openRouterModelsURL
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
				openRouterModelsURL = oldURL
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
