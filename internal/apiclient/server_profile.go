package apiclient

import (
	"context"
	"net/http"
)

// ServerProfileInfo mirrors one entry of GET /api/v1/settings/server-profile's profile list.
type ServerProfileInfo struct {
	ID             string `json:"id"`
	Mesh           bool   `json:"mesh"`
	Plugins        bool   `json:"plugins"`
	Experimental   int    `json:"experimental_features"`
	RetentionDays  int    `json:"retention_days"`
	LoopbackAgent  bool   `json:"loopback_agent"`
	LazyLoops      bool   `json:"lazy_loops"`
	Recommendation bool   `json:"posture_recommendations"`
	SBOMScan       bool   `json:"sbom_scan"`
}

// ServerProfileProblem is one reason a profile move is refused or needs a confirm.
type ServerProfileProblem struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

// ServerProfile mirrors GET /api/v1/settings/server-profile.
type ServerProfile struct {
	Active          string                 `json:"active"`
	Configured      string                 `json:"configured"`
	Source          string                 `json:"source"`
	EnvVar          string                 `json:"env_var"`
	PinnedByEnv     bool                   `json:"pinned_by_env"`
	RestartRequired bool                   `json:"restart_required"`
	Recommended     string                 `json:"recommended,omitempty"`
	Profiles        []ServerProfileInfo    `json:"profiles"`
	Problems        []ServerProfileProblem `json:"problems"`
	NeedsConfirm    bool                   `json:"needs_confirm"`
	Saved           bool                   `json:"saved"`
	UpdatedBy       string                 `json:"updated_by,omitempty"`
}

// UpdateServerProfileRequest is the PUT /api/v1/settings/server-profile body.
type UpdateServerProfileRequest struct {
	Profile string `json:"profile"`
	Confirm bool   `json:"confirm,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// GetServerProfile calls GET /api/v1/settings/server-profile.
func (c *Client) GetServerProfile(ctx context.Context) (ServerProfile, error) {
	var out ServerProfile
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/server-profile", nil, &out)
	return out, err
}

// SetServerProfile calls PUT /api/v1/settings/server-profile.
func (c *Client) SetServerProfile(ctx context.Context, req UpdateServerProfileRequest) (ServerProfile, error) {
	var out ServerProfile
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/server-profile", req, &out)
	return out, err
}
