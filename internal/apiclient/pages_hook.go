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
	return getJSON[StaticSiteHook](ctx, c, "/api/v1/static-sites/"+PathEscape(site)+"/hook")
}

// PutStaticSiteHook creates the hook, or rotates its secret when rotate is set, and returns the one-time URL.
// The server answers 409 when the intent does not match the current state.
func (c *Client) PutStaticSiteHook(ctx context.Context, site string, rotate bool) (StaticSiteHook, error) {
	var out StaticSiteHook
	var body any
	if rotate {
		body = map[string]bool{"rotate": true}
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/static-sites/"+PathEscape(site)+"/hook", body, &out)
	return out, err
}

// DeleteStaticSiteHook revokes the hook.
func (c *Client) DeleteStaticSiteHook(ctx context.Context, site string) error {
	return c.del(ctx, "/api/v1/static-sites/"+PathEscape(site)+"/hook")
}
