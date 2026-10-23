package apiclient

import (
	"context"
	"net/http"
)

// DNSProviderInfo is one built-in wildcard-DNS provider.
type DNSProviderInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Suffix   string `json:"suffix"`
	IPFormat string `json:"ip_format"`
	IPv4Only bool   `json:"ipv4_only"`
	Note     string `json:"note"`
}

// DNSProviderEffective is a provider selection.
type DNSProviderEffective struct {
	Provider         string `json:"provider"`
	CustomBaseDomain string `json:"custom_base_domain,omitempty"`
	Inherited        bool   `json:"inherited"`
}

// DNSProviderState is the wire shape of the DNS provider endpoints.
type DNSProviderState struct {
	Providers  []DNSProviderInfo    `json:"providers"`
	Default    DNSProviderEffective `json:"default"`
	Effective  DNSProviderEffective `json:"effective"`
	PublicHost string               `json:"public_host,omitempty"`
	Hostname   string               `json:"hostname,omitempty"`
	URL        string               `json:"url,omitempty"`
}

// DNSProviderRequest selects a provider; an empty Provider on an app clears its override.
type DNSProviderRequest struct {
	Provider         string `json:"provider"`
	CustomBaseDomain string `json:"custom_base_domain,omitempty"`
}

// DNSProviderFix is a suggested remedy for a failed resolution test.
type DNSProviderFix struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider,omitempty"`
}

// DNSProviderTestResult is the outcome of a resolution test.
type DNSProviderTestResult struct {
	Status     string           `json:"status"`
	Provider   string           `json:"provider"`
	Hostname   string           `json:"hostname,omitempty"`
	Expected   []string         `json:"expected,omitempty"`
	Resolved   []string         `json:"resolved,omitempty"`
	Message    string           `json:"message"`
	Fixes      []DNSProviderFix `json:"fixes,omitempty"`
	DigCommand string           `json:"dig_command,omitempty"`
}

func dnsProviderPath(app string) string {
	if app == "" {
		return "/api/v1/settings/dns-provider"
	}
	return "/api/v1/apps/" + PathEscape(app) + "/dns-provider"
}

// GetDNSProvider reads the instance default, or an app's effective choice when app is set.
func (c *Client) GetDNSProvider(ctx context.Context, app string) (DNSProviderState, error) {
	var out DNSProviderState
	err := c.do(ctx, http.MethodGet, dnsProviderPath(app), nil, &out)
	return out, err
}

// SetDNSProvider writes the instance default, or an app's override when app is set.
func (c *Client) SetDNSProvider(ctx context.Context, app string, req DNSProviderRequest) (DNSProviderState, error) {
	var out DNSProviderState
	err := c.do(ctx, http.MethodPut, dnsProviderPath(app), req, &out)
	return out, err
}

// TestDNSProvider resolves a sample hostname through the chosen (or effective) provider.
func (c *Client) TestDNSProvider(ctx context.Context, app string, req DNSProviderRequest) (DNSProviderTestResult, error) {
	var out DNSProviderTestResult
	err := c.do(ctx, http.MethodPost, dnsProviderPath(app)+"/test", req, &out)
	return out, err
}
