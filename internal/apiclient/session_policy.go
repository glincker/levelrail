package apiclient

import (
	"context"
	"net/http"
)

// SessionPolicy mirrors internal/api's sessionPolicyResource.
type SessionPolicy struct {
	TeamID      string   `json:"team_id"`
	TeamName    string   `json:"team_name"`
	IdleTimeout string   `json:"idle_timeout"`
	MaxLifetime string   `json:"max_lifetime"`
	IPAllowlist []string `json:"ip_allowlist"`
	RequireMFA  bool     `json:"require_mfa"`
	UpdatedBy   string   `json:"updated_by"`
	UpdatedAt   string   `json:"updated_at"`
}

// SessionPolicyRequest is the body of a policy save; durations use Go syntax such as 30m or 12h.
type SessionPolicyRequest struct {
	IdleTimeout string   `json:"idle_timeout"`
	MaxLifetime string   `json:"max_lifetime"`
	IPAllowlist []string `json:"ip_allowlist"`
	RequireMFA  bool     `json:"require_mfa"`
}

// EffectiveSessionPolicy is the merged policy that applies to the caller.
type EffectiveSessionPolicy struct {
	IdleTimeout string `json:"idle_timeout"`
	MaxLifetime string `json:"max_lifetime"`
	RequireMFA  bool   `json:"require_mfa"`
	AllowLists  int    `json:"allowlists"`
	YourAddress string `json:"your_address"`
	Allowed     bool   `json:"address_allowed"`
}

// ListSessionPolicies calls GET /api/v1/security/session-policies.
func (c *Client) ListSessionPolicies(ctx context.Context) ([]SessionPolicy, error) {
	var out []SessionPolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/security/session-policies", nil, &out)
	return out, err
}

// SetSessionPolicy calls PUT /api/v1/security/session-policies/{team_id}.
func (c *Client) SetSessionPolicy(ctx context.Context, teamID string, req SessionPolicyRequest) (SessionPolicy, error) {
	var out SessionPolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/security/session-policies/"+PathEscape(teamID), req, &out)
	return out, err
}

// ClearSessionPolicy calls DELETE /api/v1/security/session-policies/{team_id}.
func (c *Client) ClearSessionPolicy(ctx context.Context, teamID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/security/session-policies/"+PathEscape(teamID), nil, nil)
}

// EffectiveSessionPolicy calls GET /api/v1/security/session-policy/effective.
func (c *Client) EffectiveSessionPolicy(ctx context.Context) (EffectiveSessionPolicy, error) {
	var out EffectiveSessionPolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/security/session-policy/effective", nil, &out)
	return out, err
}
