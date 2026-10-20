package apiclient

import (
	"context"
	"net/http"
)

// SystemRestart is the /api/v1/system/restart resource.
type SystemRestart struct {
	BootID    string `json:"boot_id"`
	Supported bool   `json:"supported"`
	Pending   bool   `json:"pending"`
	Reason    string `json:"reason,omitempty"`
	Hint      string `json:"hint,omitempty"`
}

// GetSystemRestart calls GET /api/v1/system/restart.
func (c *Client) GetSystemRestart(ctx context.Context) (SystemRestart, error) {
	var out SystemRestart
	err := c.do(ctx, http.MethodGet, "/api/v1/system/restart", nil, &out)
	return out, err
}

// RestartControlPlane calls POST /api/v1/system/restart.
func (c *Client) RestartControlPlane(ctx context.Context) (SystemRestart, error) {
	var out SystemRestart
	err := c.do(ctx, http.MethodPost, "/api/v1/system/restart", nil, &out)
	return out, err
}
