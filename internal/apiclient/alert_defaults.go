package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DefaultAlert mirrors internal/api's defaultAlertResource.
type DefaultAlert struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Enabled  bool   `json:"enabled"`
	Firing   bool   `json:"firing"`
	LeadTime string `json:"lead_time,omitempty"`
}

// ListDefaultAlerts calls GET /api/v1/alert-defaults.
func (c *Client) ListDefaultAlerts(ctx context.Context) ([]DefaultAlert, error) {
	var out []DefaultAlert
	err := c.do(ctx, http.MethodGet, "/api/v1/alert-defaults", nil, &out)
	return out, err
}

// SetDefaultAlertEnabled calls PUT /api/v1/alert-defaults/{id}.
func (c *Client) SetDefaultAlertEnabled(ctx context.Context, id string, enabled bool) (DefaultAlert, error) {
	var out DefaultAlert
	err := c.do(ctx, http.MethodPut, "/api/v1/alert-defaults/"+url.PathEscape(id), map[string]bool{"enabled": enabled}, &out)
	return out, err
}

// ResetDefaultAlerts calls POST /api/v1/alert-defaults/reset.
func (c *Client) ResetDefaultAlerts(ctx context.Context) ([]DefaultAlert, error) {
	var out []DefaultAlert
	err := c.do(ctx, http.MethodPost, "/api/v1/alert-defaults/reset", nil, &out)
	return out, err
}
