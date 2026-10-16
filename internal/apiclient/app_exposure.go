package apiclient

import (
	"context"
	"net/http"
)

// AppExposureChange is one line of an exposure preview.
type AppExposureChange struct {
	Kind string `json:"kind"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// AppExposureURL is one URL an app answers on under its current exposure.
type AppExposureURL struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// AppPortHolder names who holds a host port on the node.
type AppPortHolder struct {
	Port   int    `json:"port"`
	Kind   string `json:"kind"`
	Holder string `json:"holder"`
}

// AppExposureRequest is the body of the exposure preview and set calls.
type AppExposureRequest struct {
	Mode          string `json:"mode,omitempty"`
	HostPort      *int   `json:"host_port,omitempty"`
	ContainerPort *int   `json:"container_port,omitempty"`
}

// AppExposurePlan is the preview of an exposure change.
type AppExposurePlan struct {
	Mode          string              `json:"mode"`
	ContainerPort int                 `json:"container_port"`
	HostPort      int                 `json:"host_port,omitempty"`
	BindAddress   string              `json:"bind_address"`
	Changes       []AppExposureChange `json:"changes"`
	Warnings      []string            `json:"warnings,omitempty"`
	Recreates     bool                `json:"recreates"`
	URLs          []AppExposureURL    `json:"urls"`
}

// AppExposureResource is GET /api/v1/apps/{name}/exposure.
type AppExposureResource struct {
	AppExposurePlan
	App            string          `json:"app"`
	Running        bool            `json:"running"`
	FirewallRuleID string          `json:"firewall_rule_id,omitempty"`
	PortsInUse     []AppPortHolder `json:"ports_in_use"`
	PortRange      [2]int          `json:"port_range"`
	Outcome        string          `json:"outcome,omitempty"`
	LocalNode      bool            `json:"local_node"`
}

// AppExposureProbe is one target of a reachability check.
type AppExposureProbe struct {
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Failure   string `json:"failure,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// AppExposureCheck is POST /api/v1/apps/{name}/exposure/check.
type AppExposureCheck struct {
	App     string             `json:"app"`
	Scope   string             `json:"scope"`
	Targets []AppExposureProbe `json:"targets"`
}

func exposurePath(name, suffix string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/exposure" + suffix
}

// GetAppExposure calls GET /api/v1/apps/{name}/exposure.
func (c *Client) GetAppExposure(ctx context.Context, name string) (AppExposureResource, error) {
	var out AppExposureResource
	err := c.do(ctx, http.MethodGet, exposurePath(name, ""), nil, &out)
	return out, err
}

// PreviewAppExposure calls POST /api/v1/apps/{name}/exposure/preview.
func (c *Client) PreviewAppExposure(ctx context.Context, name string, req AppExposureRequest) (AppExposurePlan, error) {
	var out AppExposurePlan
	err := c.do(ctx, http.MethodPost, exposurePath(name, "/preview"), req, &out)
	return out, err
}

// SetAppExposure calls PUT /api/v1/apps/{name}/exposure.
func (c *Client) SetAppExposure(ctx context.Context, name string, req AppExposureRequest) (AppExposureResource, error) {
	var out AppExposureResource
	err := c.do(ctx, http.MethodPut, exposurePath(name, ""), req, &out)
	return out, err
}

// CheckAppExposure calls POST /api/v1/apps/{name}/exposure/check.
func (c *Client) CheckAppExposure(ctx context.Context, name string) (AppExposureCheck, error) {
	var out AppExposureCheck
	err := c.do(ctx, http.MethodPost, exposurePath(name, "/check"), nil, &out)
	return out, err
}
