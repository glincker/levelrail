package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// PostureFix mirrors internal/api's postureFix: a dashboard link, a named
// one-click action, and an optional CLI command without the program name.
type PostureFix struct {
	Kind   string            `json:"kind"`
	Link   string            `json:"link,omitempty"`
	Action string            `json:"action,omitempty"`
	Params map[string]string `json:"params,omitempty"`
	CLI    string            `json:"cli,omitempty"`
}

// PostureItem mirrors internal/api's postureItem.
type PostureItem struct {
	ID       string      `json:"id"`
	Severity string      `json:"severity"`
	Status   string      `json:"status"`
	Count    int         `json:"count"`
	Subjects []string    `json:"subjects,omitempty"`
	Detail   string      `json:"detail,omitempty"`
	Fix      *PostureFix `json:"fix,omitempty"`
}

// PostureCounts mirrors internal/api's postureCounts.
type PostureCounts struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
	Passing  int `json:"passing"`
}

// SecurityPosture mirrors internal/api's postureResponse. Items is empty
// unless the caller is an admin (Full).
type SecurityPosture struct {
	Score       int           `json:"score"`
	Grade       string        `json:"grade"`
	GeneratedAt time.Time     `json:"generated_at"`
	Counts      PostureCounts `json:"counts"`
	Full        bool          `json:"full"`
	Items       []PostureItem `json:"items"`
	Account     []PostureItem `json:"account"`
}

// SecurityPolicy mirrors internal/api's securityPolicyResource.
type SecurityPolicy struct {
	ApprovalScope        string            `json:"approval_scope"`
	MaxTokenLifetimeDays int               `json:"max_token_lifetime_days"`
	WarnUnusedDays       int               `json:"warn_unused_days"`
	DisableUnusedDays    int               `json:"disable_unused_days"`
	Sources              map[string]string `json:"sources"`
	NewDeviceApproval    bool              `json:"new_device_approval"`
	NoticeGraceDays      int               `json:"notice_grace_days"`
}

// SecurityPolicyUpdate is the PUT body; nil fields keep their value.
type SecurityPolicyUpdate struct {
	ApprovalScope        *string `json:"approval_scope,omitempty"`
	MaxTokenLifetimeDays *int    `json:"max_token_lifetime_days,omitempty"`
	WarnUnusedDays       *int    `json:"warn_unused_days,omitempty"`
	DisableUnusedDays    *int    `json:"disable_unused_days,omitempty"`
}

// Session mirrors internal/api's sessionResource.
type Session struct {
	ID         string    `json:"id"`
	Browser    string    `json:"browser"`
	IP         string    `json:"ip"`
	Network    string    `json:"network"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

// SessionToken is the subset of an API token the sessions view shows.
type SessionToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Abilities  []string   `json:"abilities"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// SecuritySessions mirrors internal/api's securitySessionsResponse.
type SecuritySessions struct {
	UserID         string          `json:"user_id"`
	Self           bool            `json:"self"`
	Sessions       []Session       `json:"sessions"`
	TrustedDevices []TrustedDevice `json:"trusted_devices"`
	Tokens         []SessionToken  `json:"tokens"`
}

// RevokedSessions mirrors internal/api's revokeSessionsResponse.
type RevokedSessions struct {
	Revoked int `json:"revoked"`
}

func withUserID(path, userID string) string {
	if userID == "" {
		return path
	}
	return path + "?" + url.Values{"user_id": {userID}}.Encode()
}

// GetSecurityPosture calls GET /api/v1/security/posture.
func (c *Client) GetSecurityPosture(ctx context.Context) (SecurityPosture, error) {
	var out SecurityPosture
	err := c.do(ctx, http.MethodGet, "/api/v1/security/posture", nil, &out)
	return out, err
}

// GetSecurityPolicy calls GET /api/v1/security/policy.
func (c *Client) GetSecurityPolicy(ctx context.Context) (SecurityPolicy, error) {
	var out SecurityPolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/security/policy", nil, &out)
	return out, err
}

// UpdateSecurityPolicy calls PUT /api/v1/security/policy (root).
func (c *Client) UpdateSecurityPolicy(ctx context.Context, in SecurityPolicyUpdate) (SecurityPolicy, error) {
	var out SecurityPolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/security/policy", in, &out)
	return out, err
}

// ListSecuritySessions calls GET /api/v1/security/sessions; userID is
// empty for the caller's own account, another account needs root.
func (c *Client) ListSecuritySessions(ctx context.Context, userID string) (SecuritySessions, error) {
	var out SecuritySessions
	err := c.do(ctx, http.MethodGet, withUserID("/api/v1/security/sessions", userID), nil, &out)
	return out, err
}

// RevokeSecuritySession calls DELETE /api/v1/security/sessions/{id}.
func (c *Client) RevokeSecuritySession(ctx context.Context, id, userID string) error {
	return c.do(ctx, http.MethodDelete, withUserID("/api/v1/security/sessions/"+PathEscape(id), userID), nil, nil)
}

// RevokeOtherSecuritySessions calls POST /api/v1/security/sessions/revoke-others.
func (c *Client) RevokeOtherSecuritySessions(ctx context.Context, userID string) (RevokedSessions, error) {
	var out RevokedSessions
	err := c.do(ctx, http.MethodPost, withUserID("/api/v1/security/sessions/revoke-others", userID), struct{}{}, &out)
	return out, err
}
