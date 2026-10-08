package apiclient

import (
	"context"
	"net/http"
	"time"
)

// ImageAutoUpdateResource mirrors internal/api's imageAutoUpdateResource.
type ImageAutoUpdateResource struct {
	Enabled       bool       `json:"enabled"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastResult    string     `json:"last_result,omitempty"`
}

// GetImageAutoUpdate calls GET /api/v1/apps/{name}/auto-update.
func (c *Client) GetImageAutoUpdate(ctx context.Context, appName string) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/auto-update", nil, &out)
	return out, err
}

// SetImageAutoUpdate calls PUT /api/v1/apps/{name}/auto-update.
func (c *Client) SetImageAutoUpdate(ctx context.Context, appName string, enabled bool) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/auto-update", map[string]bool{"enabled": enabled}, &out)
	return out, err
}

// CheckImageAutoUpdate calls POST /api/v1/apps/{name}/auto-update/check: one
// registry check now, redeploying if the tag moved.
func (c *Client) CheckImageAutoUpdate(ctx context.Context, appName string) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(appName)+"/auto-update/check", nil, &out)
	return out, err
}
