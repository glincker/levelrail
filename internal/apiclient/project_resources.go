package apiclient

import (
	"context"
)

// ListProjectApps calls GET /api/v1/projects/{id}/apps.
func (c *Client) ListProjectApps(ctx context.Context, id string) ([]AppResource, error) {
	return getJSON[[]AppResource](ctx, c, "/api/v1/projects/"+PathEscape(id)+"/apps")
}

// ListProjectDatabases calls GET /api/v1/projects/{id}/databases.
func (c *Client) ListProjectDatabases(ctx context.Context, id string) ([]DatabaseResource, error) {
	return getJSON[[]DatabaseResource](ctx, c, "/api/v1/projects/"+PathEscape(id)+"/databases")
}
