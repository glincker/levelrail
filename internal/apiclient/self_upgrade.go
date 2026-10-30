package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// SelfUpgradeBreaking is one breaking change between two versions.
type SelfUpgradeBreaking struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Summary     string `json:"summary"`
	RequiresAck bool   `json:"requires_ack"`
}

// SelfUpgradePlan is GET /api/v1/updates/self-upgrade/plan's response.
type SelfUpgradePlan struct {
	CurrentVersion string                `json:"current_version"`
	TargetVersion  string                `json:"target_version"`
	Breaking       []SelfUpgradeBreaking `json:"breaking"`
	NotesAvailable bool                  `json:"notes_available"`
	CanApply       bool                  `json:"can_apply"`
	CannotApplyWhy string                `json:"cannot_apply_reason"`
	Command        string                `json:"command"`
	Steps          []string              `json:"steps"`
	// Schema fields are absent when the database or the target manifest is unreadable.
	DBSchemaVersion     *int     `json:"db_schema_version,omitempty"`
	TargetSchemaVersion *int     `json:"target_schema_version,omitempty"`
	PendingMigrations   *int     `json:"pending_migrations,omitempty"`
	RollbackCommands    []string `json:"rollback_commands,omitempty"`
}

// SelfUpgradeStep is one step of an attempt's timeline.
type SelfUpgradeStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	At         string `json:"at"`
	DurationMS int64  `json:"duration_ms"`
}

// SelfUpgradeLogLine is one event of an attempt's live log.
type SelfUpgradeLogLine struct {
	At   string `json:"at"`
	Step string `json:"step"`
	Line string `json:"line"`
}

// SelfUpgradeAttempt is one recorded upgrade attempt.
type SelfUpgradeAttempt struct {
	ID          string               `json:"id"`
	FromVersion string               `json:"from_version"`
	ToVersion   string               `json:"to_version"`
	FromSchema  int                  `json:"from_schema"`
	ToSchema    int                  `json:"to_schema"`
	Initiator   string               `json:"initiator"`
	Outcome     string               `json:"outcome"`
	FailedStep  string               `json:"failed_step"`
	Error       string               `json:"error"`
	BackupName  string               `json:"backup_name"`
	Acked       []string             `json:"acked"`
	Steps       []SelfUpgradeStep    `json:"steps"`
	CurrentStep string               `json:"current_step"`
	Log         []SelfUpgradeLogLine `json:"log"`
	StartedAt   string               `json:"started_at"`
	FinishedAt  string               `json:"finished_at"`

	DatabaseRestored  bool `json:"database_restored"`
	RolledBackMinutes int  `json:"rolled_back_minutes"`

	Failure *SelfUpgradeFailure `json:"failure,omitempty"`
}

// SelfUpgradeFailure is the typed cause and fix buttons of a failed attempt.
type SelfUpgradeFailure struct {
	Cause        string `json:"cause"`
	Primary      string `json:"primary"`
	FixID        string `json:"fix_id,omitempty"`
	Retryable    bool   `json:"retryable"`
	Rollbackable bool   `json:"rollbackable"`
	CLI          string `json:"cli,omitempty"`
}

// RetrySelfUpgradeAttempt calls POST .../self-upgrade/attempts/{id}/retry.
func (c *Client) RetrySelfUpgradeAttempt(ctx context.Context, id string) (SelfUpgradeStarted, error) {
	var out SelfUpgradeStarted
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/self-upgrade/attempts/"+url.PathEscape(id)+"/retry", nil, &out)
	return out, err
}

// RollbackSelfUpgradeAttempt calls POST .../self-upgrade/attempts/{id}/rollback.
func (c *Client) RollbackSelfUpgradeAttempt(ctx context.Context, id string) (RestoreStarted, error) {
	var out RestoreStarted
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/self-upgrade/attempts/"+url.PathEscape(id)+"/rollback", nil, &out)
	return out, err
}

// RestorePoint is one kept pre-upgrade restore point.
type RestorePoint struct {
	ID                    string `json:"id"`
	Version               string `json:"version"`
	Schema                int    `json:"schema"`
	CreatedAt             string `json:"created_at"`
	SizeBytes             int64  `json:"size_bytes"`
	HasBinary             bool   `json:"has_binary"`
	HasUnit               bool   `json:"has_unit"`
	DatabaseRestoreNeeded bool   `json:"database_restore_needed"`
	CanRestore            bool   `json:"can_restore"`
	CannotRestoreWhy      string `json:"cannot_restore_reason"`
}

// RestorePoints is GET /api/v1/updates/restore-points' response.
type RestorePoints struct {
	RestorePoints []RestorePoint `json:"restore_points"`
}

// RestoreStarted is POST /api/v1/updates/restore-points/{id}/restore's response.
type RestoreStarted struct {
	Status       string `json:"status"`
	RestorePoint string `json:"restore_point"`
	Version      string `json:"version"`
}

// ListRestorePoints calls GET /api/v1/updates/restore-points.
func (c *Client) ListRestorePoints(ctx context.Context) (RestorePoints, error) {
	var out RestorePoints
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/restore-points", nil, &out)
	return out, err
}

// RestoreRestorePoint calls POST /api/v1/updates/restore-points/{id}/restore.
func (c *Client) RestoreRestorePoint(ctx context.Context, id string) (RestoreStarted, error) {
	var out RestoreStarted
	body := map[string]any{"confirm": id}
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/restore-points/"+url.PathEscape(id)+"/restore", body, &out)
	return out, err
}

// SelfUpgradeAttempts is GET /api/v1/updates/self-upgrade/attempts' response.
type SelfUpgradeAttempts struct {
	Attempts []SelfUpgradeAttempt `json:"attempts"`
}

// SelfUpgradeStarted is POST /api/v1/updates/self-upgrade's response.
type SelfUpgradeStarted struct {
	Status        string `json:"status"`
	TargetVersion string `json:"target_version"`
}

// GetSelfUpgradePlan calls GET /api/v1/updates/self-upgrade/plan. An empty
// target asks for the latest release on the configured channel.
func (c *Client) GetSelfUpgradePlan(ctx context.Context, target string) (SelfUpgradePlan, error) {
	var out SelfUpgradePlan
	path := "/api/v1/updates/self-upgrade/plan"
	if target != "" {
		path += "?target=" + url.QueryEscape(target)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// StartSelfUpgrade calls POST /api/v1/updates/self-upgrade.
func (c *Client) StartSelfUpgrade(ctx context.Context, target string, ack []string) (SelfUpgradeStarted, error) {
	var out SelfUpgradeStarted
	body := map[string]any{"target": target, "ack": ack}
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/self-upgrade", body, &out)
	return out, err
}

// ListSelfUpgradeAttempts calls GET /api/v1/updates/self-upgrade/attempts.
func (c *Client) ListSelfUpgradeAttempts(ctx context.Context) (SelfUpgradeAttempts, error) {
	var out SelfUpgradeAttempts
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/self-upgrade/attempts", nil, &out)
	return out, err
}

// SelfUpgradeStatus is GET /api/v1/updates/self-upgrade/status' response.
type SelfUpgradeStatus struct {
	CurrentVersion string              `json:"current_version"`
	Running        *SelfUpgradeAttempt `json:"running"`
	LastFinished   *SelfUpgradeAttempt `json:"last_finished"`
	Verified       bool                `json:"verified"`
}

// GetSelfUpgradeStatus calls GET /api/v1/updates/self-upgrade/status.
func (c *Client) GetSelfUpgradeStatus(ctx context.Context) (SelfUpgradeStatus, error) {
	var out SelfUpgradeStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/self-upgrade/status", nil, &out)
	return out, err
}
