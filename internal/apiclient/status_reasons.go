package apiclient

import (
	"context"
	"net/http"
)

// StatusReason is one row of GET /api/v1/status-reasons.
type StatusReason struct {
	Code     string `json:"code"`
	Kind     string `json:"kind"`
	Meaning  string `json:"meaning"`
	NextStep string `json:"next_step"`
}

// ListStatusReasons calls GET /api/v1/status-reasons.
func (c *Client) ListStatusReasons(ctx context.Context) ([]StatusReason, error) {
	var out []StatusReason
	err := c.do(ctx, http.MethodGet, "/api/v1/status-reasons", nil, &out)
	return out, err
}
