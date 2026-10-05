package store

import (
	"context"
	"fmt"
	"time"
)

// ProbeAttempt is one individual readiness-probe attempt made during a
// deploy's cutover (migrations/0284_probe_attempts.sql), reported by
// internal/probe.WithOnAttempt via internal/reconcile/application's
// waitReady. ServiceName and Image are used only by RecordProbeAttempt,
// to resolve DeployAttemptID the same way RecordRollout/
// RecordRolloutFailure already do (deploy_attempt_safety.go); the table
// itself stores neither, so ListProbeAttempts always leaves both empty.
type ProbeAttempt struct {
	ID              int64
	DeployAttemptID string
	ServiceName     string
	Image           string
	Target          string
	Success         bool
	StatusCode      int
	ExitCode        int
	Error           string
	LatencyMS       int64
	ProbedAt        time.Time
}

// RecordProbeAttempt persists one probe attempt against the newest
// succeeded deploy_attempts row for (a.ServiceName, a.Image), the same
// resolution RecordRollout/RecordRolloutFailure already use since
// waitReady has no deploy_attempt_id of its own to pass in. Best-effort
// by design, matching RecordRollout's own no-match tolerance: a
// service/image pair that currently matches no such row inserts
// nothing rather than erroring.
func (db *DB) RecordProbeAttempt(ctx context.Context, a ProbeAttempt) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO probe_attempts (deploy_attempt_id, target, success, status_code, exit_code, error, latency_ms, probed_at)
		SELECT id, ?, ?, ?, ?, ?, ?, ?
		FROM deploy_attempts
		WHERE service_name = ? AND image = ? AND status = ?
		ORDER BY started_at DESC LIMIT 1
	`, a.Target, a.Success, a.StatusCode, a.ExitCode, a.Error, a.LatencyMS, a.ProbedAt.UTC().Format(time.RFC3339Nano),
		a.ServiceName, a.Image, DeployAttemptStatusSucceeded)
	if err != nil {
		return fmt.Errorf("store: record probe attempt for %q: %w", a.ServiceName, err)
	}
	return nil
}

// ListProbeAttempts returns every probe attempt recorded for deployID,
// in the order they actually happened.
func (db *DB) ListProbeAttempts(ctx context.Context, deployID string) ([]ProbeAttempt, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, deploy_attempt_id, target, success, status_code, exit_code, error, latency_ms, probed_at
		FROM probe_attempts
		WHERE deploy_attempt_id = ?
		ORDER BY probed_at ASC, id ASC
	`, deployID)
	if err != nil {
		return nil, fmt.Errorf("store: list probe attempts for %q: %w", deployID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []ProbeAttempt
	for rows.Next() {
		var (
			a        ProbeAttempt
			probedAt string
		)
		if err := rows.Scan(&a.ID, &a.DeployAttemptID, &a.Target, &a.Success, &a.StatusCode, &a.ExitCode, &a.Error, &a.LatencyMS, &probedAt); err != nil {
			return nil, fmt.Errorf("store: scan probe attempt: %w", err)
		}
		a.ProbedAt, err = time.Parse(time.RFC3339Nano, probedAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse probe attempt probed_at %q: %w", probedAt, err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate probe attempts for %q: %w", deployID, err)
	}
	return out, nil
}
