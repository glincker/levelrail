package apiclient

import (
	"context"
	"net/http"
)

// SCIMToken mirrors internal/api's scimTokenResource.
type SCIMToken struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedBy  string  `json:"created_by"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
	RevokedAt  *string `json:"revoked_at"`
}

// SCIMTokenCreated is a new or rotated token with its one-time plaintext.
type SCIMTokenCreated struct {
	SCIMToken
	Token string `json:"token"`
}

// ListSCIMTokens calls GET /api/v1/sso/scim/tokens.
func (c *Client) ListSCIMTokens(ctx context.Context) ([]SCIMToken, error) {
	var out []SCIMToken
	err := c.do(ctx, http.MethodGet, "/api/v1/sso/scim/tokens", nil, &out)
	return out, err
}

// CreateSCIMToken calls POST /api/v1/sso/scim/tokens.
func (c *Client) CreateSCIMToken(ctx context.Context, name string) (SCIMTokenCreated, error) {
	var out SCIMTokenCreated
	err := c.do(ctx, http.MethodPost, "/api/v1/sso/scim/tokens", map[string]string{"name": name}, &out)
	return out, err
}

// RotateSCIMToken calls POST /api/v1/sso/scim/tokens/{id}/rotate.
func (c *Client) RotateSCIMToken(ctx context.Context, id string) (SCIMTokenCreated, error) {
	var out SCIMTokenCreated
	err := c.do(ctx, http.MethodPost, "/api/v1/sso/scim/tokens/"+PathEscape(id)+"/rotate", nil, &out)
	return out, err
}

// RevokeSCIMToken calls DELETE /api/v1/sso/scim/tokens/{id}.
func (c *Client) RevokeSCIMToken(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/sso/scim/tokens/"+PathEscape(id), nil, nil)
}

// SetSCIMDefaultRole calls PUT /api/v1/sso/settings with the directory default role.
func (c *Client) SetSCIMDefaultRole(ctx context.Context, roleID string) (SSOSettings, error) {
	var out SSOSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/sso/settings", map[string]string{"scim_default_role_id": roleID}, &out)
	return out, err
}
