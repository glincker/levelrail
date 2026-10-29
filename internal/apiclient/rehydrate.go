package apiclient

import (
	"context"
	"net/http"
	"time"
)

// RehydrateBackup is the backup a rehydrate item restores from.
type RehydrateBackup struct {
	ID         string    `json:"id"`
	FinishedAt time.Time `json:"finished_at"`
	SizeBytes  int64     `json:"size_bytes"`
	AgeSeconds int64     `json:"age_seconds"`
}

// RehydrateItem is one resource in a whole-server recovery plan or report.
type RehydrateItem struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	App       string           `json:"app,omitempty"`
	Volume    string           `json:"volume,omitempty"`
	Database  string           `json:"database,omitempty"`
	Engine    string           `json:"engine,omitempty"`
	Status    string           `json:"status"`
	Reason    string           `json:"reason,omitempty"`
	Backup    *RehydrateBackup `json:"backup,omitempty"`
	RestoreID string           `json:"restore_id,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// RehydrateSummary counts items by outcome.
type RehydrateSummary struct {
	Total                 int   `json:"total"`
	Restorable            int   `json:"restorable"`
	Succeeded             int   `json:"succeeded"`
	Failed                int   `json:"failed"`
	Skipped               int   `json:"skipped"`
	Blocked               int   `json:"blocked"`
	WorstBackupAgeSeconds int64 `json:"worst_backup_age_seconds"`
}

// RehydrateReport is the plan or DR report of a whole-server recovery.
type RehydrateReport struct {
	ID                  string           `json:"id"`
	Mode                string           `json:"mode"`
	State               string           `json:"state"`
	GeneratedAt         time.Time        `json:"generated_at"`
	FinishedAt          *time.Time       `json:"finished_at,omitempty"`
	MaxBackupAgeSeconds int64            `json:"max_backup_age_seconds"`
	Concurrency         int              `json:"concurrency"`
	AllowStale          bool             `json:"allow_stale"`
	Items               []RehydrateItem  `json:"items"`
	Summary             RehydrateSummary `json:"summary"`
}

type rehydrateRequest struct {
	AllowStale bool `json:"allow_stale"`
}

// PlanRehydrate calls POST /api/v1/control-plane/rehydrate/plan (a dry run).
func (c *Client) PlanRehydrate(ctx context.Context, allowStale bool) (RehydrateReport, error) {
	var out RehydrateReport
	err := c.do(ctx, http.MethodPost, "/api/v1/control-plane/rehydrate/plan", rehydrateRequest{AllowStale: allowStale}, &out)
	return out, err
}

// RunRehydrate calls POST /api/v1/control-plane/rehydrate/run.
func (c *Client) RunRehydrate(ctx context.Context, allowStale bool) (RehydrateReport, error) {
	var out RehydrateReport
	err := c.do(ctx, http.MethodPost, "/api/v1/control-plane/rehydrate/run", rehydrateRequest{AllowStale: allowStale}, &out)
	return out, err
}

// GetRehydrateReport calls GET /api/v1/control-plane/rehydrate/report.
func (c *Client) GetRehydrateReport(ctx context.Context) (RehydrateReport, error) {
	var out RehydrateReport
	err := c.do(ctx, http.MethodGet, "/api/v1/control-plane/rehydrate/report", nil, &out)
	return out, err
}
