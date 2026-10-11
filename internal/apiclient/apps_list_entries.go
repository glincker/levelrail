package apiclient

import (
	"context"
	"net/http"
)

// AppListEntry is one GET /api/v1/apps row: the app plus its status rollup.
type AppListEntry struct {
	AppResource
	Status AppStatusSummary `json:"status"`
}

// ListAppEntries calls GET /api/v1/apps and keeps each row's status rollup.
func (c *Client) ListAppEntries(ctx context.Context) ([]AppListEntry, error) {
	var out []AppListEntry
	err := c.do(ctx, http.MethodGet, "/api/v1/apps", nil, &out)
	return out, err
}
