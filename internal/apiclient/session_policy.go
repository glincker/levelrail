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
	MaxSessions int      `json:"max_concurrent_sessions"`
	OnLimit     string   `json:"on_limit"`
	UpdatedBy   string   `json:"updated_by"`
	UpdatedAt   string   `json:"updated_at"`
}

// SessionPolicyRequest is the body of a policy save; durations use Go syntax such as 30m or 12h.
type SessionPolicyRequest struct {
	IdleTimeout string   `json:"idle_timeout"`
	MaxLifetime string   `json:"max_lifetime"`
	IPAllowlist []string `json:"ip_allowlist"`
	RequireMFA  bool     `json:"require_mfa"`
	MaxSessions int      `json:"max_concurrent_sessions"`
	OnLimit     string   `json:"on_limit"`
}

// EffectiveSessionPolicy is the merged policy that applies to the caller.
type EffectiveSessionPolicy struct {
	IdleTimeout string `json:"idle_timeout"`
	MaxLifetime string `json:"max_lifetime"`
	RequireMFA  bool   `json:"require_mfa"`
	MaxSessions int    `json:"max_concurrent_sessions"`
	OnLimit     string `json:"on_limit"`
	AllowLists  int    `json:"allowlists"`
	YourAddress string `json:"your_address"`
	Allowed     bool   `json:"address_allowed"`
}

// ListSessionPolicies calls GET /api/v1/security/session-policies.
func (c *Client) ListSessionPolicies(ctx context.Context) ([]SessionPolicy, error) {
	return getJSON[[]SessionPolicy](ctx, c, "/api/v1/security/session-policies")
}

// SetSessionPolicy calls PUT /api/v1/security/session-policies/{team_id}.
func (c *Client) SetSessionPolicy(ctx context.Context, teamID string, req SessionPolicyRequest) (SessionPolicy, error) {
	var out SessionPolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/security/session-policies/"+PathEscape(teamID), req, &out)
	return out, err
}

// ClearSessionPolicy calls DELETE /api/v1/security/session-policies/{team_id}.
func (c *Client) ClearSessionPolicy(ctx context.Context, teamID string) error {
	return c.del(ctx, "/api/v1/security/session-policies/"+PathEscape(teamID))
}

// EffectiveSessionPolicy calls GET /api/v1/security/session-policy/effective.
func (c *Client) EffectiveSessionPolicy(ctx context.Context) (EffectiveSessionPolicy, error) {
	return getJSON[EffectiveSessionPolicy](ctx, c, "/api/v1/security/session-policy/effective")
}
