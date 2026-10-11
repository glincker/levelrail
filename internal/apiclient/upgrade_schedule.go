package apiclient

import (
	"context"
	"net/http"
)

// UpgradeSchedule is one scheduled control plane upgrade.
type UpgradeSchedule struct {
	ID            string   `json:"id"`
	TargetVersion string   `json:"target_version"`
	Source        string   `json:"source"`
	Mode          string   `json:"mode"`
	State         string   `json:"state"`
	RunAt         string   `json:"run_at"`
	RetryUntil    string   `json:"retry_until"`
	Timezone      string   `json:"timezone"`
	Acked         []string `json:"acked"`
	MajorAcked    bool     `json:"major_acked"`
	CreatedBy     string   `json:"created_by"`
	CreatedAt     string   `json:"created_at"`
	CheckCount    int      `json:"check_count"`
	LastCheckAt   string   `json:"last_check_at"`
	LastReason    string   `json:"last_reason"`
	FiredAt       string   `json:"fired_at"`
	FinishedAt    string   `json:"finished_at"`
}

// UpgradeWindow is one opening of a maintenance window.
type UpgradeWindow struct {
	Start  string `json:"start"`
	End    string `json:"end"`
	Source string `json:"source"`
	Active bool   `json:"active"`
}

// UpgradeScheduleStatus is GET /api/v1/updates/schedule's response.
type UpgradeScheduleStatus struct {
	Available      bool              `json:"available"`
	ServerTimezone string            `json:"server_timezone"`
	Now            string            `json:"now"`
	Active         *UpgradeSchedule  `json:"active"`
	Recent         []UpgradeSchedule `json:"recent"`
	NextWindow     *UpgradeWindow    `json:"next_window"`
}

// UpgradeScheduleRequest is POST /api/v1/updates/schedule's body.
type UpgradeScheduleRequest struct {
	Target           string   `json:"target,omitempty"`
	At               string   `json:"at,omitempty"`
	AtLocal          string   `json:"at_local,omitempty"`
	Timezone         string   `json:"timezone,omitempty"`
	Window           bool     `json:"window,omitempty"`
	Ack              []string `json:"ack,omitempty"`
	AcknowledgeMajor bool     `json:"acknowledge_major,omitempty"`
	Replace          bool     `json:"replace,omitempty"`
}

// UpgradePolicy is the automatic upgrade policy.
type UpgradePolicy struct {
	Mode                  string         `json:"mode"`
	Effective             string         `json:"effective"`
	WindowCron            string         `json:"window_cron"`
	WindowDurationMinutes int            `json:"window_duration_minutes"`
	Timezone              string         `json:"timezone"`
	MinReleaseAgeDays     int            `json:"min_release_age_days"`
	NotifyChannelIDs      []string       `json:"notify_channel_ids"`
	Paused                bool           `json:"paused"`
	UpdatedBy             string         `json:"updated_by"`
	UpdatedAt             string         `json:"updated_at"`
	Decision              string         `json:"decision"`
	DecisionTag           string         `json:"decision_tag"`
	DecisionAt            string         `json:"decision_at"`
	ServerTimezone        string         `json:"server_timezone"`
	NextWindow            *UpgradeWindow `json:"next_window"`
}

// UpgradePolicyRequest is PUT /api/v1/updates/auto's body.
type UpgradePolicyRequest struct {
	Mode                  string   `json:"mode"`
	WindowCron            string   `json:"window_cron"`
	WindowDurationMinutes int      `json:"window_duration_minutes"`
	Timezone              string   `json:"timezone"`
	MinReleaseAgeDays     int      `json:"min_release_age_days"`
	NotifyChannelIDs      []string `json:"notify_channel_ids"`
	Paused                bool     `json:"paused"`
}

// UpgradeBackupNowResult is POST /api/v1/updates/backup-now's response.
type UpgradeBackupNowResult struct {
	Backup struct {
		Name      string `json:"name"`
		SizeBytes int64  `json:"size_bytes"`
		CreatedAt string `json:"created_at"`
	} `json:"backup"`
	Preflight UpdatePreflight `json:"preflight"`
}

// GetUpgradeSchedule calls GET /api/v1/updates/schedule.
func (c *Client) GetUpgradeSchedule(ctx context.Context) (UpgradeScheduleStatus, error) {
	var out UpgradeScheduleStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/schedule", nil, &out)
	return out, err
}

// CreateUpgradeSchedule calls POST /api/v1/updates/schedule.
func (c *Client) CreateUpgradeSchedule(ctx context.Context, req UpgradeScheduleRequest) (UpgradeSchedule, error) {
	var out UpgradeSchedule
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/schedule", req, &out)
	return out, err
}

// CancelUpgradeSchedule calls DELETE /api/v1/updates/schedule.
func (c *Client) CancelUpgradeSchedule(ctx context.Context) (UpgradeSchedule, error) {
	var out UpgradeSchedule
	err := c.do(ctx, http.MethodDelete, "/api/v1/updates/schedule", nil, &out)
	return out, err
}

// GetUpgradePolicy calls GET /api/v1/updates/auto.
func (c *Client) GetUpgradePolicy(ctx context.Context) (UpgradePolicy, error) {
	var out UpgradePolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/auto", nil, &out)
	return out, err
}

// SetUpgradePolicy calls PUT /api/v1/updates/auto.
func (c *Client) SetUpgradePolicy(ctx context.Context, req UpgradePolicyRequest) (UpgradePolicy, error) {
	var out UpgradePolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/updates/auto", req, &out)
	return out, err
}

// UpgradeBackupNow calls POST /api/v1/updates/backup-now.
func (c *Client) UpgradeBackupNow(ctx context.Context) (UpgradeBackupNowResult, error) {
	var out UpgradeBackupNowResult
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/backup-now", nil, &out)
	return out, err
}
