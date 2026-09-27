package apiclient

import (
	"context"
	"net/http"
)

// UpgradeCheck is one preflight result from GET /api/v1/updates/preflight.
type UpgradeCheck struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// UpdatePreflight is GET /api/v1/updates/preflight's response.
type UpdatePreflight struct {
	CurrentVersion  string         `json:"current_version"`
	LatestVersion   *string        `json:"latest_version"`
	UpdateAvailable bool           `json:"update_available"`
	ReleaseURL      *string        `json:"release_url"`
	ReleaseNotes    string         `json:"release_notes"`
	Checks          []UpgradeCheck `json:"checks"`
	Blocked         bool           `json:"blocked"`
	UpgradeCommand  string         `json:"upgrade_command"`
	RollbackCommand string         `json:"rollback_command"`
}

// GetUpdatePreflight calls GET /api/v1/updates/preflight.
func (c *Client) GetUpdatePreflight(ctx context.Context) (UpdatePreflight, error) {
	var out UpdatePreflight
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/preflight", nil, &out)
	return out, err
}
