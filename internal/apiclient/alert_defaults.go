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
	return getJSON[[]DefaultAlert](ctx, c, "/api/v1/alert-defaults")
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

// DefaultAlertSettings mirrors internal/api's alertDefaultsSettings.
type DefaultAlertSettings struct {
	Enabled           bool   `json:"enabled"`
	CrashloopRestarts int    `json:"crashloop_restarts"`
	ErrorRatePercent  int    `json:"error_rate_percent"`
	DigestSchedule    string `json:"digest_schedule"`
}

// DefaultAlertSettings calls GET /api/v1/alert-defaults/settings.
func (c *Client) DefaultAlertSettings(ctx context.Context) (DefaultAlertSettings, error) {
	return getJSON[DefaultAlertSettings](ctx, c, "/api/v1/alert-defaults/settings")
}

// SetDefaultAlertSettings calls PUT /api/v1/alert-defaults/settings with only the fields in patch.
func (c *Client) SetDefaultAlertSettings(ctx context.Context, patch map[string]any) (DefaultAlertSettings, error) {
	var out DefaultAlertSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/alert-defaults/settings", patch, &out)
	return out, err
}

// DigestPreview mirrors the weekly digest preview response.
type DigestPreview struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
}

// AlertDigestPreview calls GET /api/v1/alert-defaults/digest-preview.
func (c *Client) AlertDigestPreview(ctx context.Context) (DigestPreview, error) {
	return getJSON[DigestPreview](ctx, c, "/api/v1/alert-defaults/digest-preview")
}
