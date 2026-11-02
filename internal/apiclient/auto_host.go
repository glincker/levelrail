package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AutoHost is an app's automatic hostname setting.
type AutoHost struct {
	Enabled  bool   `json:"enabled"`
	Hostname string `json:"hostname,omitempty"`
}

// CertBudget is the certificates issued in the last week against the warning level.
type CertBudget struct {
	Issued     int       `json:"issued"`
	Warn       int       `json:"warn"`
	WindowDays int       `json:"window_days"`
	Over       bool      `json:"over"`
	CheckedAt  time.Time `json:"checked_at"`
}

// GetAutoHost calls GET /api/v1/apps/{name}/auto-host.
func (c *Client) GetAutoHost(ctx context.Context, app string) (AutoHost, error) {
	var out AutoHost
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(app)+"/auto-host", nil, &out)
	return out, err
}

// SetAutoHost calls PUT /api/v1/apps/{name}/auto-host.
func (c *Client) SetAutoHost(ctx context.Context, app string, enabled bool) (AutoHost, error) {
	var out AutoHost
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(app)+"/auto-host", map[string]bool{"enabled": enabled}, &out)
	return out, err
}

// GetCertBudget calls GET /api/v1/certificates/budget.
func (c *Client) GetCertBudget(ctx context.Context) (CertBudget, error) {
	var out CertBudget
	err := c.do(ctx, http.MethodGet, "/api/v1/certificates/budget", nil, &out)
	return out, err
}
