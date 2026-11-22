package apiclient

import (
	"context"
	"net/http"
)

// getJSON is the shared GET-and-decode shape; new getters use it instead of
// a hand written `var out T` plus c.do.
func getJSON[T any](ctx context.Context, c *Client, path string) (T, error) {
	var out T
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// del issues a DELETE that has no response body to decode.
func (c *Client) del(ctx context.Context, path string) error {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}
