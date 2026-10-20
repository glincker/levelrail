package apiclient

import (
	"context"
	"net/http"
)

// PluginTestResult is the outcome of POST /api/v1/plugins/{name}/test.
type PluginTestResult struct {
	OK      bool     `json:"ok"`
	Zones   []string `json:"zones,omitempty"`
	Message string   `json:"message"`
}

// DNSProviderPlugin is the dns_provider plugin chosen for DNS writes, empty for the built-in client.
type DNSProviderPlugin struct {
	Plugin   string `json:"plugin"`
	Provider string `json:"provider"`
}

// TestPlugin calls POST /api/v1/plugins/{name}/test (dns_provider plugins only).
func (c *Client) TestPlugin(ctx context.Context, name string) (PluginTestResult, error) {
	var out PluginTestResult
	err := c.do(ctx, http.MethodPost, "/api/v1/plugins/"+PathEscape(name)+"/test", nil, &out)
	return out, err
}

// GetDNSProviderPlugin calls GET /api/v1/settings/dns-provider-plugin.
func (c *Client) GetDNSProviderPlugin(ctx context.Context) (DNSProviderPlugin, error) {
	var out DNSProviderPlugin
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/dns-provider-plugin", nil, &out)
	return out, err
}

// SetDNSProviderPlugin calls PUT /api/v1/settings/dns-provider-plugin. An empty name selects the built-in client.
func (c *Client) SetDNSProviderPlugin(ctx context.Context, name string) (DNSProviderPlugin, error) {
	var out DNSProviderPlugin
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/dns-provider-plugin", map[string]string{"plugin": name}, &out)
	return out, err
}
