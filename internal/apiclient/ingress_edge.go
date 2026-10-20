package apiclient

import (
	"context"
	"net/http"
)

// EdgeLimitValue is one effective edge limit and where it came from (env, setting or default).
type EdgeLimitValue struct {
	Value  int64  `json:"value"`
	Source string `json:"source"`
}

// EdgeEffective is the set of limits the ingress enforces.
type EdgeEffective struct {
	MaxHeaderBytes    EdgeLimitValue `json:"max_header_bytes"`
	MaxBodyBytes      EdgeLimitValue `json:"max_body_bytes"`
	ReadHeaderTimeout EdgeLimitValue `json:"read_header_timeout_secs"`
	IdleTimeout       EdgeLimitValue `json:"idle_timeout_secs"`
}

// EdgeDetection names the proxy in front of this instance, as seen on the request.
type EdgeDetection struct {
	Proxy        string   `json:"proxy"`
	Evidence     []string `json:"evidence,omitempty"`
	PeerAddr     string   `json:"peer_addr,omitempty"`
	Suggested    string   `json:"suggested,omitempty"`
	RangesRotate bool     `json:"ranges_rotate,omitempty"`
}

// EdgeSettings is GET/PUT /api/v1/settings/ingress/edge.
type EdgeSettings struct {
	TrustedProxies       string          `json:"trusted_proxies"`
	TrustedProxiesSource string          `json:"trusted_proxies_source"`
	TrustedProxiesActive bool            `json:"trusted_proxies_active"`
	MaxHeaderBytes       *int64          `json:"max_header_bytes"`
	MaxBodyBytes         *int64          `json:"max_body_bytes"`
	ReadHeaderTimeout    *int64          `json:"read_header_timeout_secs"`
	IdleTimeout          *int64          `json:"idle_timeout_secs"`
	Effective            EdgeEffective   `json:"effective"`
	Detected             EdgeDetection   `json:"detected"`
	NeedsTrustedProxies  bool            `json:"needs_trusted_proxies"`
	Defaults             EdgeEffective   `json:"defaults"`
	EnvLocked            map[string]bool `json:"env_locked"`
}

// EdgeSettingsUpdate is the PUT body; a nil limit restores the default.
type EdgeSettingsUpdate struct {
	TrustedProxies    string `json:"trusted_proxies"`
	MaxHeaderBytes    *int64 `json:"max_header_bytes"`
	MaxBodyBytes      *int64 `json:"max_body_bytes"`
	ReadHeaderTimeout *int64 `json:"read_header_timeout_secs"`
	IdleTimeout       *int64 `json:"idle_timeout_secs"`
}

// AppIngressLimits is GET/PUT /api/v1/apps/{name}/ingress-limits.
type AppIngressLimits struct {
	MaxBodyBytes         *int64 `json:"max_body_bytes"`
	InstanceMaxBodyBytes int64  `json:"instance_max_body_bytes"`
}

const edgeSettingsPath = "/api/v1/settings/ingress/edge"

// GetEdgeSettings calls GET /api/v1/settings/ingress/edge.
func (c *Client) GetEdgeSettings(ctx context.Context) (EdgeSettings, error) {
	var out EdgeSettings
	err := c.do(ctx, http.MethodGet, edgeSettingsPath, nil, &out)
	return out, err
}

// UpdateEdgeSettings calls PUT /api/v1/settings/ingress/edge.
func (c *Client) UpdateEdgeSettings(ctx context.Context, u EdgeSettingsUpdate) (EdgeSettings, error) {
	var out EdgeSettings
	err := c.do(ctx, http.MethodPut, edgeSettingsPath, u, &out)
	return out, err
}

// ApplyDetectedProxy calls POST .../edge/trusted-proxies/apply-detected.
func (c *Client) ApplyDetectedProxy(ctx context.Context) (EdgeSettings, error) {
	var out EdgeSettings
	err := c.do(ctx, http.MethodPost, edgeSettingsPath+"/trusted-proxies/apply-detected", nil, &out)
	return out, err
}

// GetAppIngressLimits calls GET /api/v1/apps/{name}/ingress-limits.
func (c *Client) GetAppIngressLimits(ctx context.Context, app string) (AppIngressLimits, error) {
	var out AppIngressLimits
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/ingress-limits", nil, &out)
	return out, err
}

// SetAppIngressLimits calls PUT /api/v1/apps/{name}/ingress-limits; a nil cap clears the override.
func (c *Client) SetAppIngressLimits(ctx context.Context, app string, maxBodyBytes *int64) (AppIngressLimits, error) {
	var out AppIngressLimits
	body := struct {
		MaxBodyBytes *int64 `json:"max_body_bytes"`
	}{maxBodyBytes}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(app)+"/ingress-limits", body, &out)
	return out, err
}
