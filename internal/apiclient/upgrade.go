package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// UpgradeCheck is one preflight result from GET /api/v1/updates/preflight.
type UpgradeCheck struct {
	Code      string      `json:"code"`
	Name      string      `json:"name"`
	Status    string      `json:"status"`
	Message   string      `json:"message"`
	Kind      string      `json:"kind"`
	Retryable bool        `json:"retryable"`
	Protects  string      `json:"protects"`
	Hint      string      `json:"hint"`
	Fix       *UpgradeFix `json:"fix"`
}

// UpgradeFix is the action that can clear a preflight check.
type UpgradeFix struct {
	Action string `json:"action"`
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
	Waiting         bool           `json:"waiting"`
	UpgradeCommand  string         `json:"upgrade_command"`
	RollbackCommand string         `json:"rollback_command"`
	ReleaseKind     string         `json:"release_kind,omitempty"`
	Channel         string         `json:"channel,omitempty"`
	Edge            *EdgeGuidance  `json:"edge,omitempty"`

	State               string `json:"state,omitempty"`
	StateReason         string `json:"state_reason,omitempty"`
	DBSchema            *int   `json:"db_schema,omitempty"`
	LatestSchema        *int   `json:"latest_schema,omitempty"`
	MigrationsToApply   *int   `json:"migrations_to_apply,omitempty"`
	VerifyWindowSeconds int    `json:"verify_window_seconds,omitempty"`
	RollbackVersion     string `json:"rollback_version,omitempty"`
}

// EdgeGuidance is what the edge channel means for this install: a moving
// container image tag rather than a release binary.
type EdgeGuidance struct {
	Commit          string   `json:"commit,omitempty"`
	CommitURL       string   `json:"commit_url,omitempty"`
	Image           string   `json:"image"`
	RunsInContainer bool     `json:"runs_in_container"`
	Installable     bool     `json:"installable"`
	PullCommand     string   `json:"pull_command,omitempty"`
	RecreateCommand string   `json:"recreate_command,omitempty"`
	ComposeSnippet  string   `json:"compose_snippet,omitempty"`
	SwitchTo        []string `json:"switch_to,omitempty"`
}

// GetUpdatePreflight calls GET /api/v1/updates/preflight.
func (c *Client) GetUpdatePreflight(ctx context.Context) (UpdatePreflight, error) {
	var out UpdatePreflight
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/preflight", nil, &out)
	return out, err
}

// RecheckUpdatePreflight is GetUpdatePreflight bypassing the release cache.
func (c *Client) RecheckUpdatePreflight(ctx context.Context) (UpdatePreflight, error) {
	var out UpdatePreflight
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/preflight?refresh=1", nil, &out)
	return out, err
}

// ReleaseHistoryItem is one release in GET /api/v1/updates/releases.
type ReleaseHistoryItem struct {
	Version        string `json:"version"`
	URL            string `json:"url"`
	PublishedAt    string `json:"published_at"`
	Channel        string `json:"channel"`
	Running        bool   `json:"running"`
	Retained       bool   `json:"retained"`
	AssetName      string `json:"asset_name"`
	AssetAvailable bool   `json:"asset_available"`
	AssetSize      int64  `json:"asset_size"`
	Signed         bool   `json:"signed"`
	SchemaVersion  *int   `json:"schema_version"`
	SchemaSource   string `json:"schema_source"`
	Verdict        string `json:"verdict"`
	Notes          string `json:"notes"`
}

// ReleaseHistory is GET /api/v1/updates/releases' response.
type ReleaseHistory struct {
	CurrentVersion       string               `json:"current_version"`
	CurrentSchemaVersion *int                 `json:"current_schema_version"`
	Channel              string               `json:"channel"`
	View                 string               `json:"view"`
	Reachable            bool                 `json:"github_reachable"`
	Releases             []ReleaseHistoryItem `json:"releases"`
	RetainedOnly         []ReleaseHistoryItem `json:"retained_only"`
}

// GetReleaseHistory calls GET /api/v1/updates/releases.
func (c *Client) GetReleaseHistory(ctx context.Context, channel string) (ReleaseHistory, error) {
	path := "/api/v1/updates/releases"
	if channel != "" {
		path += "?channel=" + url.QueryEscape(channel)
	}
	var out ReleaseHistory
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// RollbackBackup is a restore point in a rollback plan.
type RollbackBackup struct {
	Name          string `json:"name"`
	CreatedAt     string `json:"created_at"`
	SchemaVersion int    `json:"schema_version"`
	Compatible    bool   `json:"compatible"`
}

// RollbackTarget is the release a plan returns to.
type RollbackTarget struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	SchemaSource  string `json:"schema_source"`
	Retained      bool   `json:"retained"`
}

// RollbackChange is one release's notes between the target and the running version.
type RollbackChange struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

// RollbackPlan is GET /api/v1/updates/rollback-plan's response.
type RollbackPlan struct {
	CurrentVersion       string           `json:"current_version"`
	CurrentSchemaVersion int              `json:"current_schema_version"`
	Target               RollbackTarget   `json:"target"`
	Verdict              string           `json:"verdict"`
	Steps                []string         `json:"steps"`
	Warnings             []string         `json:"warnings"`
	DowntimeSeconds      int              `json:"downtime_seconds"`
	RestoreRequired      bool             `json:"restore_required"`
	Backups              []RollbackBackup `json:"backups"`
	LossSince            *string          `json:"data_loss_since"`
	Command              string           `json:"command"`
	RestoreCommand       string           `json:"restore_command"`
	FetchCommand         string           `json:"fetch_command"`
	Checks               []UpgradeCheck   `json:"checks"`
	Blocked              bool             `json:"blocked"`
	Changes              []RollbackChange `json:"changes"`
	Notes                string           `json:"notes"`
	OneClick             bool             `json:"one_click"`
	OneClickReason       string           `json:"one_click_reason"`
}

// RollbackStarted is POST /api/v1/updates/rollback's response.
type RollbackStarted struct {
	Status        string `json:"status"`
	TargetVersion string `json:"target_version"`
}

// StartRollback calls POST /api/v1/updates/rollback for a binary-only rollback to a retained release.
func (c *Client) StartRollback(ctx context.Context, version string) (RollbackStarted, error) {
	var out RollbackStarted
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/rollback", map[string]string{"version": version, "confirm": version}, &out)
	return out, err
}

// GetRollbackPlan calls GET /api/v1/updates/rollback-plan.
func (c *Client) GetRollbackPlan(ctx context.Context, version string) (RollbackPlan, error) {
	var out RollbackPlan
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/rollback-plan?version="+url.QueryEscape(version), nil, &out)
	return out, err
}
