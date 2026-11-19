package apiclient

import (
	"context"
	"net/http"
	"time"
)

// StaticSiteHook is a Pages deploy hook; URL is set only on create and rotate.
type StaticSiteHook struct {
	Enabled   bool       `json:"enabled"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	URL       string     `json:"url,omitempty"`
}

// GetStaticSiteHook reads the hook state of a site, never its secret.
func (c *Client) GetStaticSiteHook(ctx context.Context, site string) (StaticSiteHook, error) {
	var out StaticSiteHook
	err := c.do(ctx, http.MethodGet, "/api/v1/static-sites/"+PathEscape(site)+"/hook", nil, &out)
	return out, err
}

// PutStaticSiteHook creates the hook or rotates its secret and returns the one-time URL.
func (c *Client) PutStaticSiteHook(ctx context.Context, site string) (StaticSiteHook, error) {
	var out StaticSiteHook
	err := c.do(ctx, http.MethodPost, "/api/v1/static-sites/"+PathEscape(site)+"/hook", nil, &out)
	return out, err
}

// DeleteStaticSiteHook revokes the hook.
func (c *Client) DeleteStaticSiteHook(ctx context.Context, site string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/static-sites/"+PathEscape(site)+"/hook", nil, nil)
}
