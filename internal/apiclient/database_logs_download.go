package apiclient

import (
	"context"
	"net/url"
	"time"
)

// DownloadDatabaseLogs calls GET /api/v1/databases/{name}/logs/download and
// returns the plain-text export for the window.
func (c *Client) DownloadDatabaseLogs(ctx context.Context, name string, from, to time.Time, q string) ([]byte, error) {
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if q != "" {
		query.Set("q", q)
	}
	return c.downloadRaw(ctx, "/api/v1/databases/"+PathEscape(name)+"/logs/download?"+query.Encode())
}
