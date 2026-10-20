package apiclient

import (
	"context"
	"net/http"
)

// ListAppImportSessions calls GET /api/v1/migration/apps/sessions; items are left out.
func (c *Client) ListAppImportSessions(ctx context.Context) ([]AppImportView, error) {
	var out struct {
		Sessions []AppImportView `json:"sessions"`
	}
	err := c.do(ctx, http.MethodGet, appImportPath+"/sessions", nil, &out)
	return out.Sessions, err
}
