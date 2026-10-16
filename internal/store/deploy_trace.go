package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Deploy trace step statuses.
const (
	TraceStatusRunning = "running"
	TraceStatusDone    = "done"
	TraceStatusFailed  = "failed"
	TraceStatusSkipped = "skipped"
	// TraceStatusWarning is a step that finished but flagged something, such
	// as a probe attempt that failed while the container was still starting.
	TraceStatusWarning = "warning"
)

// DeployTraceStep is one step of a deploy attempt's pipeline trace.
type DeployTraceStep struct {
	AttemptID  string
	Key        string
	Step       string
	Service    string
	Status     string
	Reason     string
	Detail     string
	Attempts   int
	StartedAt  time.Time
	EndedAt    time.Time
	DurationMS int64
}

// SaveDeployTraceStep inserts s, or overwrites the row with the same
// (attempt, key): status, reason and detail change, started_at is kept and
// attempts counts how often the step was written.
func (db *DB) SaveDeployTraceStep(ctx context.Context, s DeployTraceStep) error {
	if s.Key == "" {
		s.Key = s.Step
	}
	ended := ""
	if !s.EndedAt.IsZero() {
		ended = s.EndedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_trace_steps (attempt_id, step_key, step, service, status, reason, detail, started_at, ended_at, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (attempt_id, step_key) DO UPDATE SET
			status = excluded.status, reason = excluded.reason, detail = excluded.detail,
			ended_at = excluded.ended_at,
			duration_ms = CASE
				WHEN excluded.duration_ms = 0 AND excluded.ended_at != ''
				THEN CAST((julianday(excluded.ended_at) - julianday(deploy_trace_steps.started_at)) * 86400000 AS INTEGER)
				ELSE excluded.duration_ms END,
			attempts = attempts + 1
	`, s.AttemptID, s.Key, s.Step, s.Service, s.Status, s.Reason, s.Detail,
		s.StartedAt.UTC().Format(time.RFC3339Nano), ended, s.DurationMS)
	if err != nil {
		return fmt.Errorf("store: save deploy trace step %q for %q: %w", s.Key, s.AttemptID, err)
	}
	return nil
}

// ListDeployTraceSteps returns attemptID's steps in the order they first ran.
func (db *DB) ListDeployTraceSteps(ctx context.Context, attemptID string) ([]DeployTraceStep, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT attempt_id, step_key, step, service, status, reason, detail, attempts, started_at, ended_at, duration_ms
		FROM deploy_trace_steps WHERE attempt_id = ? ORDER BY id ASC
	`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy trace steps for %q: %w", attemptID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []DeployTraceStep
	for rows.Next() {
		var (
			s              DeployTraceStep
			started, ended string
		)
		if err := rows.Scan(&s.AttemptID, &s.Key, &s.Step, &s.Service, &s.Status, &s.Reason, &s.Detail, &s.Attempts, &started, &ended, &s.DurationMS); err != nil {
			return nil, fmt.Errorf("store: scan deploy trace step: %w", err)
		}
		s.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
		if ended != "" {
			s.EndedAt, _ = time.Parse(time.RFC3339Nano, ended)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate deploy trace steps: %w", err)
	}
	return out, nil
}

// TraceTargetFor resolves the newest attempt for (service, image), as
// RecordProbeAttempt does. sealed means its summary is done, so a later restart leaves the trace alone.
func (db *DB) TraceTargetFor(ctx context.Context, serviceName, image string) (attemptID string, sealed bool, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT a.id, EXISTS (
			SELECT 1 FROM deploy_trace_steps t
			WHERE t.attempt_id = a.id AND t.step_key = ? AND t.status = ?
		)
		FROM deploy_attempts a
		WHERE a.service_name = ? AND a.image = ? AND a.status IN (?, ?)
		ORDER BY a.started_at DESC LIMIT 1
	`, TraceSummaryKey, TraceStatusDone, serviceName, image, DeployAttemptStatusRunning, DeployAttemptStatusSucceeded).Scan(&attemptID, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: resolve trace target for %q: %w", serviceName, err)
	}
	return attemptID, sealed, nil
}

// TraceSummaryKey is the step key of a deploy's closing summary.
const TraceSummaryKey = "deploy_summary"
