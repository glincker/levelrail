package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// ProxyDetection is what was read from the container holding ports 80/443.
type ProxyDetection struct {
	Kind            string   `json:"kind"`
	Container       string   `json:"container"`
	Image           string   `json:"image"`
	PublishedPorts  []int    `json:"published_ports"`
	DynamicDir      string   `json:"dynamic_dir"`
	EntrypointHTTP  string   `json:"entrypoint_http"`
	EntrypointHTTPS string   `json:"entrypoint_https"`
	CertResolver    string   `json:"cert_resolver"`
	UpstreamHost    string   `json:"upstream_host"`
	Complete        bool     `json:"complete"`
	Missing         []string `json:"missing"`
}

// ProxyIntegrationSettings is PUT /api/v1/settings/proxy-integration's body.
type ProxyIntegrationSettings struct {
	Integration     string `json:"integration"`
	DynamicDir      string `json:"dynamic_dir"`
	EntrypointHTTP  string `json:"entrypoint_http"`
	EntrypointHTTPS string `json:"entrypoint_https"`
	CertResolver    string `json:"cert_resolver"`
	UpstreamHost    string `json:"upstream_host"`
}

// ProxyIngress is how this instance's ingress is set up for the proxy.
type ProxyIngress struct {
	HTTPPort              int    `json:"http_port"`
	HTTPSPort             int    `json:"https_port"`
	DashboardAddr         string `json:"dashboard_addr"`
	TLSTerminatedUpstream bool   `json:"tls_terminated_upstream"`
	PublicHTTPSPort       int    `json:"public_https_port"`
}

// ProxyCertificate is the certificate the proxy served for a domain.
type ProxyCertificate struct {
	Issuer   string `json:"issuer"`
	NotAfter string `json:"not_after"`
	Valid    bool   `json:"valid"`
}

// ProxyDomain is one managed route.
type ProxyDomain struct {
	Domain      string            `json:"domain"`
	Target      string            `json:"target"`
	App         string            `json:"app"`
	File        string            `json:"file"`
	State       string            `json:"state"`
	ProxyLoaded *bool             `json:"proxy_loaded"`
	Reachable   bool              `json:"reachable"`
	Certificate *ProxyCertificate `json:"certificate"`
	LastError   string            `json:"last_error"`
	CheckedAt   string            `json:"checked_at"`
}

// ProxyStep is one item of the setup checklist.
type ProxyStep struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// ProxyIntegration is GET /api/v1/system/proxy-integration.
type ProxyIntegration struct {
	Detected ProxyDetection           `json:"detected"`
	Settings ProxyIntegrationSettings `json:"settings"`
	Ingress  ProxyIngress             `json:"ingress"`
	Domains  []ProxyDomain            `json:"domains"`
	Steps    []ProxyStep              `json:"steps"`
	// DryRun and Changes are set by setup only.
	DryRun  bool     `json:"dry_run,omitempty"`
	Changes []string `json:"changes,omitempty"`
}

// ProxySetupRequest is POST /api/v1/system/proxy-integration/setup's body.
type ProxySetupRequest struct {
	Confirm    bool   `json:"confirm"`
	DynamicDir string `json:"dynamic_dir,omitempty"`
}

// GetProxyIntegration returns detection, settings and per-domain status.
func (c *Client) GetProxyIntegration(ctx context.Context) (ProxyIntegration, error) {
	var out ProxyIntegration
	err := c.do(ctx, http.MethodGet, "/api/v1/system/proxy-integration", nil, &out)
	return out, err
}

// SetupProxyIntegration runs setup; without Confirm it is a dry run.
func (c *Client) SetupProxyIntegration(ctx context.Context, req ProxySetupRequest) (ProxyIntegration, error) {
	var out ProxyIntegration
	err := c.do(ctx, http.MethodPost, "/api/v1/system/proxy-integration/setup", req, &out)
	return out, err
}

// ApplyProxyIntegration rewrites the managed route files now.
func (c *Client) ApplyProxyIntegration(ctx context.Context) (ProxyIntegration, error) {
	var out ProxyIntegration
	err := c.do(ctx, http.MethodPost, "/api/v1/system/proxy-integration/apply", nil, &out)
	return out, err
}

// VerifyProxyIntegration probes every route, or only domain when set.
func (c *Client) VerifyProxyIntegration(ctx context.Context, domain string) (ProxyIntegration, error) {
	path := "/api/v1/system/proxy-integration/verify"
	if domain != "" {
		path += "?" + url.Values{"domain": {domain}}.Encode()
	}
	var out ProxyIntegration
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}

// UpdateProxyIntegrationSettings replaces the settings.
func (c *Client) UpdateProxyIntegrationSettings(ctx context.Context, s ProxyIntegrationSettings) (ProxyIntegrationSettings, error) {
	var out ProxyIntegrationSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/proxy-integration", s, &out)
	return out, err
}
