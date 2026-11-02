package apiclient

import (
	"context"
	"net/http"
	"time"
)

// IngressCloudflare mirrors /api/v1/settings/ingress/cloudflare.
type IngressCloudflare struct {
	Preset          string     `json:"preset"`
	Source          string     `json:"source"`
	Ranges          int        `json:"ranges"`
	RangesSource    string     `json:"ranges_source"`
	LastRefresh     *time.Time `json:"last_refresh,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	RefreshInterval string     `json:"refresh_interval"`
}

// GetIngressCloudflare calls GET /api/v1/settings/ingress/cloudflare.
func (c *Client) GetIngressCloudflare(ctx context.Context) (IngressCloudflare, error) {
	var out IngressCloudflare
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/ingress/cloudflare", nil, &out)
	return out, err
}

// SetIngressCloudflare calls PUT /api/v1/settings/ingress/cloudflare.
func (c *Client) SetIngressCloudflare(ctx context.Context, preset string, confirm bool) (IngressCloudflare, error) {
	var out IngressCloudflare
	body := map[string]any{"preset": preset, "confirm_weakening": confirm}
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/ingress/cloudflare", body, &out)
	return out, err
}
