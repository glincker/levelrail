package apiclient

import (
	"context"
	"net/http"
)

// AppHealthResource mirrors internal/api's appHealthResource.
type AppHealthResource struct {
	Name   string         `json:"name"`
	Health *ServiceHealth `json:"health"`
}

// GetAppHealth calls GET /api/v1/apps/{name}/health.
func (c *Client) GetAppHealth(ctx context.Context, name string) (AppHealthResource, error) {
	var out AppHealthResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/health", nil, &out)
	return out, err
}

// SetAppHealth calls PUT /api/v1/apps/{name}/health, replacing name's
// readiness and liveness config as a whole.
func (c *Client) SetAppHealth(ctx context.Context, name string, health ServiceHealth) (AppHealthResource, error) {
	var out AppHealthResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/health", health, &out)
	return out, err
}

// ClearAppHealth calls DELETE /api/v1/apps/{name}/health.
func (c *Client) ClearAppHealth(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/health", nil, nil)
}
