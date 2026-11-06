package apiclient

import "context"

// DownloadDeployReceipt returns one deploy's receipt as raw bytes; format is "json" or "md".
func (c *Client) DownloadDeployReceipt(ctx context.Context, name, deployID, format string) ([]byte, error) {
	path := "/api/v1/apps/" + PathEscape(name) + "/deployments/" + PathEscape(deployID) + "/receipt"
	if format != "" {
		path += "?format=" + format
	}
	return c.downloadRaw(ctx, path)
}
