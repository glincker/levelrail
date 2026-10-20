package apiclient

import (
	"context"
	"net/http"
)

// AIControlResource mirrors GET /api/v1/settings/ai-control.
type AIControlResource struct {
	Mode            string   `json:"mode"`
	AllowedEnvKinds []string `json:"allowed_env_kinds"`
	EnvKinds        []string `json:"env_kinds"`
	AdminAvailable  bool     `json:"admin_available"`
	AgentTokenCount int      `json:"agent_token_count"`
	CallerIsAgent   bool     `json:"caller_is_agent"`
	UpdatedAt       string   `json:"updated_at"`
	UpdatedBy       string   `json:"updated_by"`
}

// UpdateAIControlRequest is the PUT /api/v1/settings/ai-control body.
type UpdateAIControlRequest struct {
	Mode            string   `json:"mode"`
	AllowedEnvKinds []string `json:"allowed_env_kinds"`
}

// RevokeAgentTokensResource mirrors the revoke-agent-tokens response.
type RevokeAgentTokensResource struct {
	Revoked int `json:"revoked"`
}

// GetAIControl calls GET /api/v1/settings/ai-control.
func (c *Client) GetAIControl(ctx context.Context) (AIControlResource, error) {
	var out AIControlResource
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/ai-control", nil, &out)
	return out, err
}

// UpdateAIControl calls PUT /api/v1/settings/ai-control.
func (c *Client) UpdateAIControl(ctx context.Context, req UpdateAIControlRequest) (AIControlResource, error) {
	var out AIControlResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/ai-control", req, &out)
	return out, err
}

// RevokeAgentTokens calls POST /api/v1/settings/ai-control/revoke-agent-tokens.
func (c *Client) RevokeAgentTokens(ctx context.Context) (RevokeAgentTokensResource, error) {
	var out RevokeAgentTokensResource
	err := c.do(ctx, http.MethodPost, "/api/v1/settings/ai-control/revoke-agent-tokens", nil, &out)
	return out, err
}
