package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// NodePool mirrors internal/store's NodePool.
type NodePool struct {
	Name     string            `json:"name"`
	Selector map[string]string `json:"selector"`
}

// WarmTargets mirrors internal/store's WarmTargets.
type WarmTargets struct {
	Nodes  []string          `json:"nodes"`
	Labels map[string]string `json:"labels"`
}

// ListNodePools calls GET /api/v1/node-pools.
func (c *Client) ListNodePools(ctx context.Context) ([]NodePool, error) {
	out, err := getJSON[struct {
		Pools []NodePool `json:"pools"`
	}](ctx, c, "/api/v1/node-pools")
	return out.Pools, err
}

// SetNodePool calls PUT /api/v1/node-pools/{name}.
func (c *Client) SetNodePool(ctx context.Context, name string, selector map[string]string) (NodePool, error) {
	var out NodePool
	err := c.do(ctx, http.MethodPut, "/api/v1/node-pools/"+url.PathEscape(name), map[string]map[string]string{"selector": selector}, &out)
	return out, err
}

// DeleteNodePool calls DELETE /api/v1/node-pools/{name}.
func (c *Client) DeleteNodePool(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/node-pools/"+url.PathEscape(name), nil, nil)
}

// SetAppPool calls PUT /api/v1/apps/{name}/pool; an empty pool clears it.
func (c *Client) SetAppPool(ctx context.Context, app, pool string) (PlacementResource, error) {
	var out PlacementResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+url.PathEscape(app)+"/pool", map[string]string{"pool": pool}, &out)
	return out, err
}

// GetAppWarm calls GET /api/v1/apps/{name}/warm.
func (c *Client) GetAppWarm(ctx context.Context, app string) (WarmTargets, error) {
	return getJSON[WarmTargets](ctx, c, "/api/v1/apps/"+url.PathEscape(app)+"/warm")
}

// SetAppWarm calls PUT /api/v1/apps/{name}/warm.
func (c *Client) SetAppWarm(ctx context.Context, app string, t WarmTargets) (WarmTargets, error) {
	var out WarmTargets
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+url.PathEscape(app)+"/warm", t, &out)
	return out, err
}
