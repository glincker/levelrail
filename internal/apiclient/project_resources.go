package apiclient

import (
	"context"
	"net/http"
)

// ListProjectApps calls GET /api/v1/projects/{id}/apps.
func (c *Client) ListProjectApps(ctx context.Context, id string) ([]AppResource, error) {
	var out []AppResource
	err := c.do(ctx, http.MethodGet, "/api/v1/projects/"+PathEscape(id)+"/apps", nil, &out)
	return out, err
}

// ListProjectDatabases calls GET /api/v1/projects/{id}/databases.
func (c *Client) ListProjectDatabases(ctx context.Context, id string) ([]DatabaseResource, error) {
	var out []DatabaseResource
	err := c.do(ctx, http.MethodGet, "/api/v1/projects/"+PathEscape(id)+"/databases", nil, &out)
	return out, err
}
