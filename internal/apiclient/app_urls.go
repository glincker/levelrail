package apiclient

import (
	"context"
	"net/http"
)

// AppURLAddress is one way to reach an app, from GET /api/v1/apps/{name}/urls.
type AppURLAddress struct {
	Kind          string `json:"kind"`
	Host          string `json:"host,omitempty"`
	URL           string `json:"url,omitempty"`
	Port          int    `json:"port,omitempty"`
	ReachableHint string `json:"reachable_hint"`
}

// AppURLs is the response of GET /api/v1/apps/{name}/urls.
type AppURLs struct {
	App           string          `json:"app"`
	ContainerPort int             `json:"container_port"`
	HostPort      int             `json:"host_port,omitempty"`
	BindAddress   string          `json:"bind_address"`
	Running       bool            `json:"running"`
	Node          string          `json:"node,omitempty"`
	LocalNode     bool            `json:"local_node"`
	Addresses     []AppURLAddress `json:"addresses"`
}

// GetAppURLs calls GET /api/v1/apps/{name}/urls.
func (c *Client) GetAppURLs(ctx context.Context, app string) (AppURLs, error) {
	var out AppURLs
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/urls", nil, &out)
	return out, err
}
