package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DNSTokenResource mirrors internal/api's dnsTokenResource. The credential never appears here.
type DNSTokenResource struct {
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
	HasKey   bool   `json:"has_key"`
}

// UpdateDNSTokenRequest mirrors internal/api's updateDNSTokenRequest. An empty Credential keeps the stored one.
type UpdateDNSTokenRequest struct {
	Enabled    bool   `json:"enabled"`
	Credential string `json:"credential,omitempty"`
}

// DNSTokenTestResult mirrors internal/api's dnsTokenTestResponse.
type DNSTokenTestResult struct {
	OK    bool     `json:"ok"`
	Zones []string `json:"zones,omitempty"`
	Error string   `json:"error,omitempty"`
}

func dnsTokenPath(provider string) string {
	return "/api/v1/settings/dns-providers/" + url.PathEscape(provider)
}

// GetDNSToken calls GET /api/v1/settings/dns-providers/{provider}.
func (c *Client) GetDNSToken(ctx context.Context, provider string) (DNSTokenResource, error) {
	return getJSON[DNSTokenResource](ctx, c, dnsTokenPath(provider))
}

// SetDNSToken calls PUT /api/v1/settings/dns-providers/{provider}.
func (c *Client) SetDNSToken(ctx context.Context, provider string, req UpdateDNSTokenRequest) (DNSTokenResource, error) {
	var out DNSTokenResource
	err := c.do(ctx, http.MethodPut, dnsTokenPath(provider), req, &out)
	return out, err
}

// DisconnectDNSToken calls DELETE /api/v1/settings/dns-providers/{provider}.
func (c *Client) DisconnectDNSToken(ctx context.Context, provider string) (DNSTokenResource, error) {
	var out DNSTokenResource
	err := c.do(ctx, http.MethodDelete, dnsTokenPath(provider), nil, &out)
	return out, err
}

// TestDNSToken calls POST /api/v1/settings/dns-providers/{provider}/test.
func (c *Client) TestDNSToken(ctx context.Context, provider string) (DNSTokenTestResult, error) {
	var out DNSTokenTestResult
	err := c.do(ctx, http.MethodPost, dnsTokenPath(provider)+"/test", nil, &out)
	return out, err
}
