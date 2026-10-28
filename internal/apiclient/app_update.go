package apiclient

import (
	"context"
	"net/http"
)

// UpdateApp calls PUT /api/v1/apps/{name} with the app, returning the
// saved app. The server keeps fields the body omits; callers should still
// start from GetApp and change only the fields they mean to.
func (c *Client) UpdateApp(ctx context.Context, name string, app AppResource) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name), app, &out)
	return out, err
}
