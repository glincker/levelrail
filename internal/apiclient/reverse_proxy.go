package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ReverseProxyHolder is a container publishing port 80 or 443.
type ReverseProxyHolder struct {
	Port           int    `json:"port"`
	Container      string `json:"container"`
	Image          string `json:"image"`
	Kind           string `json:"kind,omitempty"`
	NetworkGateway string `json:"network_gateway,omitempty"`
}

// ReverseProxyPlan is the configuration that puts the dashboard behind a proxy.
type ReverseProxyPlan struct {
	Domain      string   `json:"domain"`
	Proxy       string   `json:"proxy"`
	UpstreamURL string   `json:"upstream_url"`
	NeedsRebind bool     `json:"needs_rebind"`
	Rebind      string   `json:"rebind,omitempty"`
	Snippet     string   `json:"snippet"`
	SnippetPath string   `json:"snippet_path,omitempty"`
	Steps       []string `json:"steps"`
}

// ReverseProxyCheck is the result of the reachability check.
type ReverseProxyCheck struct {
	ResolvesToHost bool   `json:"resolves_to_host"`
	Reachable      bool   `json:"reachable"`
	Detail         string `json:"detail,omitempty"`
}

// ReverseProxyGuide is GET /api/v1/system/reverse-proxy.
type ReverseProxyGuide struct {
	Holders  []ReverseProxyHolder `json:"holders"`
	Detected string               `json:"detected_proxy,omitempty"`
	Plan     *ReverseProxyPlan    `json:"plan,omitempty"`
	Check    *ReverseProxyCheck   `json:"check,omitempty"`
}

// GetReverseProxyGuide calls GET /api/v1/system/reverse-proxy.
func (c *Client) GetReverseProxyGuide(ctx context.Context, domain, proxy string, verify bool) (ReverseProxyGuide, error) {
	q := url.Values{}
	if domain != "" {
		q.Set("domain", domain)
	}
	if proxy != "" {
		q.Set("proxy", proxy)
	}
	if verify {
		q.Set("verify", "true")
	}
	path := "/api/v1/system/reverse-proxy"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out ReverseProxyGuide
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
