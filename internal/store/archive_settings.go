package store

import (
	"context"
	"fmt"
	"time"
)

// ArchiveSettings is the single-row archive policy.
type ArchiveSettings struct {
	Enabled         bool      `json:"enabled"`
	AuditHotDays    int       `json:"audit_hot_days"`
	DeployHotDays   int       `json:"deploy_hot_days"`
	PipelineHotDays int       `json:"pipeline_hot_days"`
	KeepDays        int       `json:"keep_days"`
	LastRunAt       time.Time `json:"last_run_at,omitzero"`
	LastRunError    string    `json:"last_run_error,omitempty"`
}

// GetArchiveSettings returns the archive policy.
func (db *DB) GetArchiveSettings(ctx context.Context) (ArchiveSettings, error) {
	var (
		s       ArchiveSettings
		enabled int
		lastRun string
	)
	err := db.QueryRowContext(ctx, `SELECT enabled, audit_hot_days, deploy_hot_days, pipeline_hot_days, keep_days, last_run_at, last_run_error
		FROM archive_settings WHERE id = 1`).Scan(&enabled, &s.AuditHotDays, &s.DeployHotDays, &s.PipelineHotDays, &s.KeepDays, &lastRun, &s.LastRunError)
	if err != nil {
		return ArchiveSettings{}, fmt.Errorf("store: get archive settings: %w", err)
	}
	s.Enabled = enabled != 0
	if t, perr := time.Parse(time.RFC3339Nano, lastRun); perr == nil {
		s.LastRunAt = t
	}
	return s, nil
}

// UpdateArchiveSettings replaces the operator-controlled fields only.
func (db *DB) UpdateArchiveSettings(ctx context.Context, s ArchiveSettings, now time.Time) error {
	en := 0
	if s.Enabled {
		en = 1
	}
	_, err := db.ExecContext(ctx, `UPDATE archive_settings SET enabled = ?, audit_hot_days = ?, deploy_hot_days = ?, pipeline_hot_days = ?, keep_days = ?, updated_at = ? WHERE id = 1`,
		en, s.AuditHotDays, s.DeployHotDays, s.PipelineHotDays, s.KeepDays, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: update archive settings: %w", err)
	}
	return nil
}

// RecordArchiveRun stores the outcome of the latest archive run.
func (db *DB) RecordArchiveRun(ctx context.Context, at time.Time, runErr string) error {
	_, err := db.ExecContext(ctx, `UPDATE archive_settings SET last_run_at = ?, last_run_error = ? WHERE id = 1`, at.UTC().Format(time.RFC3339Nano), runErr)
	if err != nil {
		return fmt.Errorf("store: record archive run: %w", err)
	}
	return nil
}
