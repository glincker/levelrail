package apiclient

import (
	"context"
	"net/http"
)

// MeshCheck mirrors one readiness check of GET /api/v1/mesh/settings.
type MeshCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// MeshSettings mirrors GET /api/v1/mesh/settings.
type MeshSettings struct {
	Configured      bool        `json:"configured"`
	Effective       bool        `json:"effective"`
	Source          string      `json:"source"`
	EnvVar          string      `json:"env_var"`
	RestartRequired bool        `json:"restart_required"`
	RestartHint     string      `json:"restart_hint,omitempty"`
	RemoteNodes     int         `json:"remote_nodes"`
	Checks          []MeshCheck `json:"checks"`
	CanEnable       bool        `json:"can_enable"`
	StartError      string      `json:"start_error,omitempty"`
	UpdatedBy       string      `json:"updated_by,omitempty"`
}

// UpdateMeshSettingsRequest is the PUT /api/v1/mesh/settings body.
type UpdateMeshSettingsRequest struct {
	Enabled bool `json:"enabled"`
	Force   bool `json:"force,omitempty"`
}

// GetMeshSettings calls GET /api/v1/mesh/settings.
func (c *Client) GetMeshSettings(ctx context.Context) (MeshSettings, error) {
	var out MeshSettings
	err := c.do(ctx, http.MethodGet, "/api/v1/mesh/settings", nil, &out)
	return out, err
}

// UpdateMeshSettings calls PUT /api/v1/mesh/settings.
func (c *Client) UpdateMeshSettings(ctx context.Context, req UpdateMeshSettingsRequest) (MeshSettings, error) {
	var out MeshSettings
	err := c.do(ctx, http.MethodPut, "/api/v1/mesh/settings", req, &out)
	return out, err
}
