package apiclient

import (
	"context"
	"net/http"
)

// ProxyInstallFile is one file the install writes.
type ProxyInstallFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ProxyInstallMount is one bind mount of the proxy container.
type ProxyInstallMount struct {
	Host      string `json:"host"`
	Container string `json:"container"`
	ReadOnly  bool   `json:"read_only"`
}

// ProxyInstallPlan is the exact install the server would run.
type ProxyInstallPlan struct {
	Provider  string              `json:"provider"`
	Container string              `json:"container"`
	Image     string              `json:"image"`
	Ports     []int               `json:"ports"`
	Mounts    []ProxyInstallMount `json:"mounts"`
	Files     []ProxyInstallFile  `json:"files"`
	Restart   string              `json:"restart"`
	RouteDir  string              `json:"route_dir"`
}

// ProxyInstallRequest is POST .../proxy-integration/install's body.
type ProxyInstallRequest struct {
	Provider string `json:"provider,omitempty"`
	Confirm  bool   `json:"confirm"`
}

// ProxyInstallResult is the dry-run plan, or the outcome once confirmed.
type ProxyInstallResult struct {
	DryRun      bool              `json:"dry_run"`
	Plan        ProxyInstallPlan  `json:"plan"`
	Outcome     string            `json:"outcome,omitempty"`
	Detail      string            `json:"detail,omitempty"`
	Integration *ProxyIntegration `json:"integration,omitempty"`
}

// InstallProxy plans (Confirm false) or runs the proxy install.
func (c *Client) InstallProxy(ctx context.Context, req ProxyInstallRequest) (ProxyInstallResult, error) {
	var out ProxyInstallResult
	err := c.do(ctx, http.MethodPost, "/api/v1/system/proxy-integration/install", req, &out)
	return out, err
}
