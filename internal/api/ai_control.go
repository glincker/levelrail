package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

type aiControlResource struct {
	Mode            string   `json:"mode"`
	AllowedEnvKinds []string `json:"allowed_env_kinds"`
	EnvKinds        []string `json:"env_kinds"`
	AdminAvailable  bool     `json:"admin_available"`
	AgentTokenCount int      `json:"agent_token_count"`
	UpdatedAt       string   `json:"updated_at"`
	UpdatedBy       string   `json:"updated_by"`
}

type updateAIControlRequest struct {
	Mode            string   `json:"mode"`
	AllowedEnvKinds []string `json:"allowed_env_kinds"`
}

type revokeAgentTokensResponse struct {
	Revoked int `json:"revoked"`
}

func (rt *Router) aiControlResource(r *http.Request, s store.AIControlSettings) (aiControlResource, error) {
	count, err := rt.aiControl.CountAgentTokens(r.Context(), AIAssistantTokenName, AIAssistantAgentName)
	if err != nil {
		return aiControlResource{}, fmt.Errorf("api: ai control: %w", err)
	}
	kinds := s.AllowedEnvKinds
	if kinds == nil {
		kinds = []string{}
	}
	return aiControlResource{
		Mode: s.Mode, AllowedEnvKinds: kinds, EnvKinds: store.EnvironmentKinds,
		AdminAvailable:  experimental.Enabled(experimental.AIControl),
		AgentTokenCount: count, UpdatedAt: s.UpdatedAt, UpdatedBy: s.UpdatedBy,
	}, nil
}

// handleGetAIControl handles GET /api/v1/settings/ai-control.
func (rt *Router) handleGetAIControl(w http.ResponseWriter, r *http.Request) {
	s, err := rt.aiControl.GetAIControlSettings(r.Context())
	if err == nil {
		var res aiControlResource
		if res, err = rt.aiControlResource(r, s); err == nil {
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	rt.logger.Error("api: get ai control failed", slog.String("error", err.Error()))
	writeError(w, http.StatusInternalServerError, "internal error")
}

// handleUpdateAIControl handles PUT /api/v1/settings/ai-control.
func (rt *Router) handleUpdateAIControl(w http.ResponseWriter, r *http.Request) {
	var req updateAIControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !store.ValidAIControlMode(req.Mode) {
		writeError(w, http.StatusBadRequest, "mode must be one of off, observe, operate, admin")
		return
	}
	if req.Mode == store.AIControlModeAdmin && !experimental.Enabled(experimental.AIControl) {
		writeError(w, http.StatusBadRequest, "admin mode requires the experimental feature ai-control to be enabled")
		return
	}
	for _, k := range req.AllowedEnvKinds {
		if !slices.Contains(store.EnvironmentKinds, k) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown environment kind %q", k))
			return
		}
	}
	kinds := slices.Compact(slices.Sorted(slices.Values(req.AllowedEnvKinds)))
	_, _, actorName, _ := rt.currentActor(r)
	if err := rt.aiControl.UpdateAIControlSettings(r.Context(), req.Mode, kinds, actorName); err != nil {
		rt.logger.Error("api: update ai control failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.aiControlCache.invalidate()
	rt.handleGetAIControl(w, r)
}

// handleRevokeAgentTokens handles POST /api/v1/settings/ai-control/revoke-agent-tokens.
func (rt *Router) handleRevokeAgentTokens(w http.ResponseWriter, r *http.Request) {
	n, err := rt.aiControl.RevokeAgentTokens(r.Context(), AIAssistantTokenName, AIAssistantAgentName)
	if err != nil {
		rt.logger.Error("api: revoke agent tokens failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, revokeAgentTokensResponse{Revoked: n})
}

// MintAIAssistantToken mints the root token the in-app assistant calls the API with, labeled so the AI control gate and the audit log treat it as an agent.
func MintAIAssistantToken(ctx context.Context, tokens TokenStore) (string, error) {
	plaintext, _, err := MintAgentAPIToken(ctx, tokens, AIAssistantTokenName, []string{AbilityRoot}, nil, agentIdentity{Name: AIAssistantAgentName}, "")
	return plaintext, err
}
