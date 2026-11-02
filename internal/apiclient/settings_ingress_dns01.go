package apiclient

import (
	"context"
	"net/http"
)

// IngressDNS01 mirrors /api/v1/settings/ingress/dns01; source is stored or default.
type IngressDNS01 struct {
	Enabled           bool   `json:"enabled"`
	Source            string `json:"source"`
	ProviderConnected bool   `json:"provider_connected"`
}

// GetIngressDNS01 calls GET /api/v1/settings/ingress/dns01.
func (c *Client) GetIngressDNS01(ctx context.Context) (IngressDNS01, error) {
	var out IngressDNS01
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/ingress/dns01", nil, &out)
	return out, err
}

// SetIngressDNS01 calls PUT /api/v1/settings/ingress/dns01.
func (c *Client) SetIngressDNS01(ctx context.Context, enabled bool) (IngressDNS01, error) {
	var out IngressDNS01
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/ingress/dns01", map[string]bool{"enabled": enabled}, &out)
	return out, err
}
