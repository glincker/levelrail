package apiclient

import "context"

// CloudRequest calls one /api/v1/cloud-accounts path and decodes the JSON answer into out (nil to discard).
func (c *Client) CloudRequest(ctx context.Context, method, path string, body, out any) error {
	return c.do(ctx, method, "/api/v1/cloud-accounts"+path, body, out)
}
