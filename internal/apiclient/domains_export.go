package apiclient

import "context"

// ExportDomainsCSV calls GET /api/v1/domains/export and returns the CSV bytes.
func (c *Client) ExportDomainsCSV(ctx context.Context) ([]byte, error) {
	return c.downloadRaw(ctx, "/api/v1/domains/export?format=csv")
}
