package apiclient

import (
	"context"
	"net/http"
	"time"
)

// SiteDeployToken is one deploy token of a Pages site; Token is set only on create.
type SiteDeployToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Token      string     `json:"token,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func siteTokensPath(site string) string {
	return "/api/v1/static-sites/" + PathEscape(site) + "/deploy-tokens"
}

// CreateSiteDeployToken mints a token that can only deploy the site; expiresDays 0 uses the server default.
func (c *Client) CreateSiteDeployToken(ctx context.Context, site, label string, expiresDays int) (SiteDeployToken, error) {
	var out SiteDeployToken
	err := c.do(ctx, http.MethodPost, siteTokensPath(site), map[string]any{"name": label, "expires_in_days": expiresDays}, &out)
	return out, err
}

// ListSiteDeployTokens lists the site's deploy tokens without secrets.
func (c *Client) ListSiteDeployTokens(ctx context.Context, site string) ([]SiteDeployToken, error) {
	var out []SiteDeployToken
	err := c.do(ctx, http.MethodGet, siteTokensPath(site), nil, &out)
	return out, err
}

// RevokeSiteDeployToken revokes one deploy token of the site.
func (c *Client) RevokeSiteDeployToken(ctx context.Context, site, id string) error {
	return c.do(ctx, http.MethodDelete, siteTokensPath(site)+"/"+PathEscape(id), nil, nil)
}
