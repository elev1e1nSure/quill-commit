package watcher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"quill-commit/ai"
	"quill-commit/config"
	gitcontext "quill-commit/context"
)

// ContextManager builds AI requests and manages the static/dynamic context
// cache budget. It is a no-op when IncludeContext is false.
type ContextManager struct {
	cfg           config.Config
	apiKey        string
	static        gitcontext.Static
	staticBudget  int
	fullBudget    int
	sessionID     string
	explicitCache bool
	cacheMisses   int
}

// newContextManager creates a ContextManager. When IncludeContext is true it
// builds static context, resolves cache capability, and generates a session ID.
func newContextManager(ctx context.Context, cfg config.Config, apiKey string, repoRoot string) *ContextManager {
	cm := &ContextManager{cfg: cfg, apiKey: apiKey}
	if !cfg.IncludeContext {
		return cm
	}

	var err error
	cm.static, err = gitcontext.BuildStatic(repoRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warn: context.BuildStatic:", err)
	}

	if cfg.SessionID != "" {
		cm.sessionID = cfg.SessionID
	} else {
		cm.sessionID = generateSessionID()
	}

	cm.explicitCache, err = ai.CacheCapabilityFn(cfg.Model, apiKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warn: CacheCapability:", err)
		cm.explicitCache = false
	}

	cm.staticBudget = cfg.ContextBudget
	cm.fullBudget = cfg.ContextBudget
	return cm
}

// BuildRequest creates an ai.Request for the given diff. The returned error,
// if non-nil, is a non-fatal dynamic-context build error that the caller should
// log but not fail on.
func (cm *ContextManager) BuildRequest(ctx context.Context, diff string) (ai.Request, error) {
	var sysPrompt string
	var userPrompt string
	var dynErr error

	if cm.cfg.IncludeContext {
		var dyn gitcontext.Dynamic
		dyn, dynErr = gitcontext.BuildDynamic(cm.cfg.RecentCommitsCount)
		sysPrompt = ai.PromptForStrategy(cm.cfg.Strategy) + "\n\n---\n\n" + gitcontext.RenderSystem(cm.static, cm.staticBudget)
		userPrompt = gitcontext.RenderUser(dyn, diff)
	} else {
		sysPrompt = ai.PromptForStrategy(cm.cfg.Strategy)
		userPrompt = diff
	}

	req := ai.Request{
		SystemPrompt:  sysPrompt,
		UserPrompt:    userPrompt,
		Model:         cm.cfg.Model,
		APIKey:        cm.apiKey,
		SessionID:     cm.sessionID,
		ExplicitCache: cm.explicitCache,
		Ctx:           ctx,
	}
	return req, dynErr
}

// UpdateBudget adjusts the static budget based on cache hit/miss behavior.
func (cm *ContextManager) UpdateBudget(usage ai.Usage) {
	if !cm.cfg.IncludeContext {
		return
	}
	if usage.CachedTokens > 0 {
		if cm.cacheMisses > 0 || cm.staticBudget < cm.fullBudget {
			cm.cacheMisses = 0
			cm.staticBudget = cm.fullBudget
		}
	} else {
		cm.cacheMisses++
		if cm.cacheMisses >= 3 && cm.staticBudget > 800 {
			cm.staticBudget = 800
			if cm.staticBudget > cm.fullBudget {
				cm.staticBudget = cm.fullBudget
			}
			cm.cacheMisses = 0
		}
	}
}

func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintln(os.Stderr, "warn: generate session_id:", err)
		now := time.Now().UnixNano()
		for i := 0; i < 8; i++ {
			b[i] = byte(now >> (i * 8))
		}
		pid := os.Getpid()
		for i := 0; i < 4; i++ {
			b[8+i] = byte(pid >> (i * 8))
		}
	}
	return hex.EncodeToString(b)
}
