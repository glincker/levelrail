package apiclient

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// JanitorPolicy is the cleanup configuration.
type JanitorPolicy struct {
	Enabled               bool     `json:"enabled"`
	DryRun                bool     `json:"dry_run"`
	IntervalMinutes       int      `json:"interval_minutes"`
	DiskFreeBudgetPercent float64  `json:"disk_free_budget_percent"`
	BuildCacheBudgetMB    int64    `json:"build_cache_budget_mb"`
	BuildCacheMaxAgeHours int      `json:"build_cache_max_age_hours"`
	VolumeQuarantineDays  int      `json:"volume_quarantine_days"`
	DeleteOrphanVolumes   bool     `json:"delete_orphan_volumes"`
	AttemptRetentionDays  int      `json:"attempt_retention_days"`
	AttemptKeepPerApp     int      `json:"attempt_keep_per_app"`
	DisabledStages        []string `json:"disabled_stages"`
}

// JanitorPolicyPatch changes only the fields that are set.
type JanitorPolicyPatch struct {
	Enabled               *bool     `json:"enabled,omitempty"`
	DryRun                *bool     `json:"dry_run,omitempty"`
	IntervalMinutes       *int      `json:"interval_minutes,omitempty"`
	DiskFreeBudgetPercent *float64  `json:"disk_free_budget_percent,omitempty"`
	BuildCacheBudgetMB    *int64    `json:"build_cache_budget_mb,omitempty"`
	BuildCacheMaxAgeHours *int      `json:"build_cache_max_age_hours,omitempty"`
	VolumeQuarantineDays  *int      `json:"volume_quarantine_days,omitempty"`
	DeleteOrphanVolumes   *bool     `json:"delete_orphan_volumes,omitempty"`
	AttemptRetentionDays  *int      `json:"attempt_retention_days,omitempty"`
	AttemptKeepPerApp     *int      `json:"attempt_keep_per_app,omitempty"`
	DisabledStages        *[]string `json:"disabled_stages,omitempty"`
}

// JanitorItem is one thing a stage removed, would remove, or kept.
type JanitorItem struct {
	Name      string `json:"name"`
	Detail    string `json:"detail,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	Count     int    `json:"count,omitempty"`
	Action    string `json:"action"`
}

// JanitorStage is what one stage did.
type JanitorStage struct {
	Stage          string        `json:"stage"`
	Status         string        `json:"status"`
	Reason         string        `json:"reason,omitempty"`
	Items          []JanitorItem `json:"items"`
	ReclaimedBytes int64         `json:"reclaimed_bytes"`
	Error          string        `json:"error,omitempty"`
}

// JanitorReport is one cleanup pass.
type JanitorReport struct {
	ID             string         `json:"id"`
	StartedAt      time.Time      `json:"started_at"`
	Trigger        string         `json:"trigger"`
	DryRun         bool           `json:"dry_run"`
	FreePercent    float64        `json:"free_percent"`
	Pressure       bool           `json:"pressure"`
	Stages         []JanitorStage `json:"stages"`
	ReclaimedBytes int64          `json:"reclaimed_bytes"`
	Error          string         `json:"error,omitempty"`
}

// JanitorQuarantined is a volume held before deletion.
type JanitorQuarantined struct {
	Name          string    `json:"name"`
	SizeBytes     int64     `json:"size_bytes"`
	FirstSeen     time.Time `json:"first_seen"`
	DeletableAt   time.Time `json:"deletable_at"`
	DeleteEnabled bool      `json:"delete_enabled"`
	Ready         bool      `json:"ready"`
}

// JanitorStatus is the policy, disk state and last pass.
type JanitorStatus struct {
	Policy         JanitorPolicy        `json:"policy"`
	FreePercent    float64              `json:"free_percent"`
	FreeBytes      int64                `json:"free_bytes"`
	TotalBytes     int64                `json:"total_bytes"`
	Pressure       bool                 `json:"pressure"`
	LastRun        *JanitorReport       `json:"last_run,omitempty"`
	Quarantine     []JanitorQuarantined `json:"quarantine"`
	TotalReclaimed int64                `json:"total_reclaimed_bytes"`
}

// GetJanitor calls GET /api/v1/system/janitor.
func (c *Client) GetJanitor(ctx context.Context) (JanitorStatus, error) {
	var out JanitorStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/system/janitor", nil, &out)
	return out, err
}

// ListJanitorRuns calls GET /api/v1/system/janitor/runs.
func (c *Client) ListJanitorRuns(ctx context.Context, limit int) ([]JanitorReport, error) {
	var out []JanitorReport
	err := c.do(ctx, http.MethodGet, "/api/v1/system/janitor/runs?limit="+strconv.Itoa(limit), nil, &out)
	return out, err
}

// UpdateJanitorPolicy calls PUT /api/v1/system/janitor/policy.
func (c *Client) UpdateJanitorPolicy(ctx context.Context, patch JanitorPolicyPatch) (JanitorPolicy, error) {
	var out JanitorPolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/system/janitor/policy", patch, &out)
	return out, err
}

// RunJanitor calls POST /api/v1/system/janitor/run. dryRun previews; force
// false waits for disk pressure before the size-driven stages run.
func (c *Client) RunJanitor(ctx context.Context, dryRun, force bool) (JanitorReport, error) {
	var out JanitorReport
	path := "/api/v1/system/janitor/run?dry_run=" + strconv.FormatBool(dryRun) + "&force=" + strconv.FormatBool(force)
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}
