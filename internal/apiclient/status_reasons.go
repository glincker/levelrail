package apiclient

import (
	"context"
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
	return getJSON[[]StatusReason](ctx, c, "/api/v1/status-reasons")
}
