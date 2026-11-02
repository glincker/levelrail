package apiclient

import "context"

// ExportApp calls GET /api/v1/apps/{name}/export?format=compose and returns
// the tar stream of docker-compose.yml, .env.example and README.md.
func (c *Client) ExportApp(ctx context.Context, name, format string) ([]byte, error) {
	return c.downloadRaw(ctx, "/api/v1/apps/"+PathEscape(name)+"/export?format="+PathEscape(format))
}
