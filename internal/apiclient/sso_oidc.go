package apiclient

import (
	"context"
	"net/http"
)

// OAuthSSOPolicy mirrors the API's oauthSSOPolicyResource.
type OAuthSSOPolicy struct {
	Provider        string   `json:"provider"`
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	GroupsSupported bool     `json:"groups_supported"`
	GroupsClaim     string   `json:"groups_claim"`
	SyncRoles       bool     `json:"sync_roles"`
	DefaultRoleID   string   `json:"default_role_id"`
	JITProvisioning bool     `json:"jit_provisioning"`
	AllowedDomains  []string `json:"allowed_domains"`
}

// OAuthSSOPolicyRequest mirrors the API's oauthSSOPolicyRequest.
type OAuthSSOPolicyRequest struct {
	GroupsClaim     string   `json:"groups_claim"`
	SyncRoles       bool     `json:"sync_roles"`
	DefaultRoleID   string   `json:"default_role_id"`
	JITProvisioning bool     `json:"jit_provisioning"`
	AllowedDomains  []string `json:"allowed_domains"`
}

// OAuthSSOEvaluateRequest mirrors the API's oauthSSOEvaluateRequest.
type OAuthSSOEvaluateRequest struct {
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	Groups        []string `json:"groups"`
}

// OAuthSSOFix mirrors the API's ssoFix.
type OAuthSSOFix struct {
	Kind   string `json:"kind"`
	Action string `json:"action,omitempty"`
	Value  string `json:"value,omitempty"`
}

// OAuthSSOIssue mirrors the API's ssoIssue.
type OAuthSSOIssue struct {
	Code string      `json:"code"`
	Fix  OAuthSSOFix `json:"fix"`
}

// OAuthSSOEvaluation mirrors the API's ssoEvaluation.
type OAuthSSOEvaluation struct {
	Provider        string          `json:"provider"`
	Email           string          `json:"email"`
	EmailVerified   bool            `json:"email_verified"`
	GroupsClaim     string          `json:"groups_claim"`
	GroupsFound     bool            `json:"groups_found"`
	Groups          []string        `json:"groups"`
	GroupLikeClaims []string        `json:"group_like_claims"`
	RoleID          string          `json:"role_id"`
	RoleSource      string          `json:"role_source"`
	AccountState    string          `json:"account_state"`
	SyncRoles       bool            `json:"sync_roles"`
	Allowed         bool            `json:"allowed"`
	RefusalCode     string          `json:"refusal_code,omitempty"`
	Issues          []OAuthSSOIssue `json:"issues"`
}

// ListOAuthSSOPolicies calls GET /api/v1/sso/oidc.
func (c *Client) ListOAuthSSOPolicies(ctx context.Context) ([]OAuthSSOPolicy, error) {
	var out []OAuthSSOPolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/oidc", nil, &out)
	return out, err
}

// GetOAuthSSOPolicy calls GET /api/v1/sso/oidc/{provider}.
func (c *Client) GetOAuthSSOPolicy(ctx context.Context, provider string) (OAuthSSOPolicy, error) {
	var out OAuthSSOPolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/oidc/"+PathEscape(provider), nil, &out)
	return out, err
}

// SetOAuthSSOPolicy calls PUT /api/v1/sso/oidc/{provider}.
func (c *Client) SetOAuthSSOPolicy(ctx context.Context, provider string, req OAuthSSOPolicyRequest) (OAuthSSOPolicy, error) {
	var out OAuthSSOPolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/sso/oidc/"+PathEscape(provider), req, &out)
	return out, err
}

// EvaluateOAuthSSO calls POST /api/v1/sso/oidc/{provider}/evaluate.
func (c *Client) EvaluateOAuthSSO(ctx context.Context, provider string, req OAuthSSOEvaluateRequest) (OAuthSSOEvaluation, error) {
	var out OAuthSSOEvaluation
	err := c.do(ctx, http.MethodPost, "/api/v1/sso/oidc/"+PathEscape(provider)+"/evaluate", req, &out)
	return out, err
}
