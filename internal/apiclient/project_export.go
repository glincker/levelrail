package apiclient

import "context"

// ExportProject calls GET /api/v1/projects/{id}/export and returns the tar
// stream. includeSecrets adds the opt-in .env and needs write:sensitive.
func (c *Client) ExportProject(ctx context.Context, project string, includeSecrets bool) ([]byte, error) {
	path := "/api/v1/projects/" + PathEscape(project) + "/export"
	if includeSecrets {
		path += "?include_secrets=true"
	}
	return c.downloadRaw(ctx, path)
}
