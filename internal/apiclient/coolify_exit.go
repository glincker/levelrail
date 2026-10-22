package apiclient

import (
	"context"
	"net/http"
	"net/url"

	"github.com/GLINCKER/levelrail/internal/coolifyexit"
)

// CoolifyExitReport calls POST /api/v1/migration/coolify/exit-report. The source token goes in the body only.
func (c *Client) CoolifyExitReport(ctx context.Context, req PlatformImportRequest) (coolifyexit.Report, error) {
	var out coolifyexit.Report
	err := c.do(ctx, http.MethodPost, "/api/v1/migration/coolify/exit-report", req, &out)
	return out, err
}

// AppImportExitReport calls GET /api/v1/migration/apps/sessions/{id}/exit-report.
func (c *Client) AppImportExitReport(ctx context.Context, id string) (coolifyexit.Report, error) {
	var out coolifyexit.Report
	err := c.do(ctx, http.MethodGet, appImportPath+"/sessions/"+url.PathEscape(id)+"/exit-report", nil, &out)
	return out, err
}
