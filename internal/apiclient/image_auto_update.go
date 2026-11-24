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
	HasWebhook    bool       `json:"has_webhook"`
	// HasSigningSecret is true when GitHub package deliveries must be signed.
	HasSigningSecret bool   `json:"has_signing_secret"`
	WindowCron       string `json:"window_cron,omitempty"`
	WindowMinutes    int    `json:"window_minutes,omitempty"`
	VersionRange     string `json:"version_range,omitempty"`
}

// GetImageAutoUpdate calls GET /api/v1/apps/{name}/auto-update.
func (c *Client) GetImageAutoUpdate(ctx context.Context, appName string) (ImageAutoUpdateResource, error) {
	return getJSON[ImageAutoUpdateResource](ctx, c, "/api/v1/apps/"+PathEscape(appName)+"/auto-update")
}

// SetImageAutoUpdate calls PUT /api/v1/apps/{name}/auto-update.
func (c *Client) SetImageAutoUpdate(ctx context.Context, appName string, enabled bool) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/auto-update", map[string]bool{"enabled": enabled}, &out)
	return out, err
}

// SetImageAutoUpdateWindow calls PUT /api/v1/apps/{name}/auto-update with an
// apply window; an empty cron clears it.
func (c *Client) SetImageAutoUpdateWindow(ctx context.Context, appName string, enabled bool, cron string, minutes int) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	body := map[string]any{"enabled": enabled, "window_cron": cron, "window_minutes": minutes}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/auto-update", body, &out)
	return out, err
}

// SetImageAutoUpdateRange calls PUT /api/v1/apps/{name}/auto-update with a
// semver range such as "1.x"; an empty range clears it.
func (c *Client) SetImageAutoUpdateRange(ctx context.Context, appName string, enabled bool, versionRange string) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	body := map[string]any{"enabled": enabled, "version_range": versionRange}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/auto-update", body, &out)
	return out, err
}

// CheckImageAutoUpdate calls POST /api/v1/apps/{name}/auto-update/check: one
// registry check now that only reports a newer image; apply deploys it.
func (c *Client) CheckImageAutoUpdate(ctx context.Context, appName string, apply bool) (ImageAutoUpdateResource, error) {
	var out ImageAutoUpdateResource
	path := "/api/v1/apps/" + PathEscape(appName) + "/auto-update/check"
	if apply {
		path += "?apply=true"
	}
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}

// ImageUpdateWebhookResource is the freshly minted webhook; Token is shown once.
type ImageUpdateWebhookResource struct {
	Path  string `json:"path"`
	Token string `json:"token"`
	// SigningSecret is the GitHub webhook secret, shown once; empty when the control plane has no secrets store.
	SigningSecret string `json:"signing_secret,omitempty"`
}

// RotateImageUpdateWebhook calls POST /api/v1/apps/{name}/auto-update/webhook.
func (c *Client) RotateImageUpdateWebhook(ctx context.Context, appName string) (ImageUpdateWebhookResource, error) {
	var out ImageUpdateWebhookResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(appName)+"/auto-update/webhook", nil, &out)
	return out, err
}
