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
	// The server keeps an omitted env, so the request always carries one: an
	// empty map clears the last key. AppResource.Env stays omitempty so tool
	// output schemas never see null.
	env := app.Env
	if env == nil {
		env = map[string]string{}
	}
	body := struct {
		AppResource
		Env map[string]string `json:"env"`
	}{app, env}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name), body, &out)
	return out, err
}
