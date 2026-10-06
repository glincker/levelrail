package api

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AIAssistantTokenName and AIAssistantAgentName identify the internal token the in-app assistant holds.
const (
	AIAssistantTokenName = "AI Assistant (internal)"
	AIAssistantAgentName = "ai-assistant"
)

const (
	aiControlCacheTTL   = 5 * time.Second
	aiControlKindCustom = "custom"

	msgAgentDisabled    = "agent access is disabled by an administrator"
	msgAgentModeLimit   = "agent access is limited by the current AI control mode"
	msgAgentEnvLimit    = "agent access is not allowed in this environment"
	msgAgentNoApprove   = "agents cannot approve deploys, a human must"
	msgAgentNoSelfAdmin = "agents cannot change AI control settings"
)

var (
	agentResourcePath = regexp.MustCompile(`^/api/v1/(apps|databases)/([^/]+)(/|$)`)
	agentApprovePaths = []*regexp.Regexp{
		regexp.MustCompile(`^/api/v1/deploy-approvals/[^/]+/approve$`),
		regexp.MustCompile(`^/api/v1/apps/[^/]+/pipeline-runs/[^/]+/approvals/[^/]+$`),
		regexp.MustCompile(`^/api/v1/apps/[^/]+/previews/[^/]+/approve$`),
	}
)

// AIControlStore is the store surface the AI control gate and settings routes need.
type AIControlStore interface {
	GetAIControlSettings(ctx context.Context) (store.AIControlSettings, error)
	UpdateAIControlSettings(ctx context.Context, mode string, kinds []string, updatedBy string) error
	RevokeAgentTokens(ctx context.Context, internalName, internalAgent string) (int, error)
	CountAgentTokens(ctx context.Context, internalName, internalAgent string) (int, error)
	EnvironmentOfApp(ctx context.Context, appName string) (*store.EnvironmentRef, error)
	EnvironmentOfDatabase(ctx context.Context, databaseName string) (*store.EnvironmentRef, error)
}

type aiControlCache struct {
	mu       sync.Mutex
	settings store.AIControlSettings
	loadedAt time.Time
	valid    bool
}

func (c *aiControlCache) get() (store.AIControlSettings, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.valid && time.Since(c.loadedAt) < aiControlCacheTTL {
		return c.settings, true
	}
	return store.AIControlSettings{}, false
}

func (c *aiControlCache) put(s store.AIControlSettings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.settings, c.loadedAt, c.valid = s, time.Now(), true
}

func (c *aiControlCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valid = false
}

func (rt *Router) aiControlSettings(ctx context.Context) (store.AIControlSettings, error) {
	if s, ok := rt.aiControlCache.get(); ok {
		return s, nil
	}
	s, err := rt.aiControl.GetAIControlSettings(ctx)
	if err != nil {
		return store.AIControlSettings{}, err
	}
	rt.aiControlCache.put(s)
	return s, nil
}

// agentRequest is the part of a request the AI control decision depends on.
type agentRequest struct {
	Ability, Method, Path string
	ResourceKind          string
	ResourceName          string
}

func newAgentRequest(r *http.Request, required string) agentRequest {
	req := agentRequest{Ability: required, Method: r.Method, Path: r.URL.Path}
	if m := agentResourcePath.FindStringSubmatch(r.URL.Path); m != nil {
		req.ResourceKind = strings.TrimSuffix(m[1], "s")
		req.ResourceName = r.PathValue("name")
		if req.ResourceName == "" {
			req.ResourceName = m[2]
		}
	}
	return req
}

// evaluateAgentAccess decides one agent class request. Admin mode without the ai-control flag behaves as operate.
func evaluateAgentAccess(s store.AIControlSettings, adminEnabled bool, req agentRequest, kindOf func(resourceKind, name string) (string, error)) (bool, string, error) {
	mode := s.Mode
	if mode == store.AIControlModeAdmin && !adminEnabled {
		mode = store.AIControlModeOperate
	}
	if mode != store.AIControlModeOperate && mode != store.AIControlModeAdmin {
		if mode == store.AIControlModeObserve {
			if req.Ability == AbilityRead || req.Ability == AbilityReadSensitive {
				return true, "", nil
			}
			return false, msgAgentModeLimit, nil
		}
		return false, msgAgentDisabled, nil
	}
	if req.Method != http.MethodGet && strings.HasPrefix(req.Path, "/api/v1/settings/ai-control") {
		return false, msgAgentNoSelfAdmin, nil
	}
	if req.Method == http.MethodPost && slices.ContainsFunc(agentApprovePaths, func(p *regexp.Regexp) bool { return p.MatchString(req.Path) }) {
		return false, msgAgentNoApprove, nil
	}
	if mode == store.AIControlModeOperate && req.Ability == AbilityRoot {
		return false, msgAgentModeLimit, nil
	}
	if req.ResourceKind == "" {
		return true, "", nil
	}
	kind, err := kindOf(req.ResourceKind, req.ResourceName)
	if err != nil {
		return false, "", err
	}
	if !slices.Contains(s.AllowedEnvKinds, kind) {
		return false, msgAgentEnvLimit, nil
	}
	return true, "", nil
}

func (rt *Router) agentEnvironmentKind(ctx context.Context, resourceKind, name string) (string, error) {
	var ref *store.EnvironmentRef
	var err error
	if resourceKind == "database" {
		ref, err = rt.aiControl.EnvironmentOfDatabase(ctx, name)
	} else {
		ref, err = rt.aiControl.EnvironmentOfApp(ctx, name)
	}
	if err != nil {
		return "", err
	}
	if ref == nil || ref.Kind == "" {
		return aiControlKindCustom, nil
	}
	return ref.Kind, nil
}

// aiControlRejects applies the AI control mode to an agent class request, writes the rejection and audits it, and reports whether the request was stopped.
func (rt *Router) aiControlRejects(w http.ResponseWriter, r *http.Request, rec *store.APIToken, required string) bool {
	if rec.AgentName == "" || rt.aiControl == nil {
		return false
	}
	ctx := r.Context()
	s, err := rt.aiControlSettings(ctx)
	var allowed bool
	var reason string
	if err == nil {
		kindOf := func(k, n string) (string, error) { return rt.agentEnvironmentKind(ctx, k, n) }
		allowed, reason, err = evaluateAgentAccess(s, experimental.Enabled(experimental.AIControl), newAgentRequest(r, required), kindOf)
	}
	if err != nil {
		rt.logger.Error("api: ai control decision failed", slog.String("error", err.Error()), slog.String("token_id", rec.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return true
	}
	if allowed {
		return false
	}
	rt.recordAudit(withAgentName(ctx, rec.AgentName), r, required, "token", rec.ID, rec.Name, http.StatusForbidden)
	writeError(w, http.StatusForbidden, reason)
	return true
}

// aiControlRejectsPinned applies the gate to a session link minted from an agent token.
func (rt *Router) aiControlRejectsPinned(w http.ResponseWriter, r *http.Request, originTokenID, required string) bool {
	rec, err := rt.tokens.GetAPITokenByID(r.Context(), originTokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return true
	}
	return rt.aiControlRejects(w, r, rec, required)
}
