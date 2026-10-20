package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DeployExitLine is one captured output line of an exited container.
type DeployExitLine struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

// DeployExit is a release container that stopped before it could serve
// (migrations/0760_deploy_exit_captures.sql).
type DeployExit struct {
	ServiceName string
	AttemptID   string
	Image       string
	Container   string
	ExitCode    int
	OOMKilled   bool
	Reason      string
	Message     string
	StartedAt   time.Time
	FinishedAt  time.Time
	LogTail     []DeployExitLine
	CapturedAt  time.Time
}

// RecordDeployExit stores e against the newest attempt that deployed
// e.Image, replacing an earlier capture for that attempt so a crash loop
// keeps only its latest exit.
func (db *DB) RecordDeployExit(ctx context.Context, e DeployExit) error {
	attemptID := e.AttemptID
	if attemptID == "" {
		err := db.QueryRowContext(ctx, `
			SELECT id FROM deploy_attempts WHERE service_name = ? AND image = ?
			ORDER BY started_at DESC LIMIT 1
		`, e.ServiceName, e.Image).Scan(&attemptID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("store: record deploy exit for %q: find attempt: %w", e.ServiceName, err)
		}
	}
	lines := e.LogTail
	if lines == nil {
		lines = []DeployExitLine{}
	}
	tail, err := json.Marshal(lines)
	if err != nil {
		return fmt.Errorf("store: record deploy exit for %q: encode log tail: %w", e.ServiceName, err)
	}
	captured := e.CapturedAt
	if captured.IsZero() {
		captured = time.Now()
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO deploy_exit_captures (service_name, attempt_id, image, container, exit_code, oom_killed,
			reason, message, started_at, finished_at, log_tail, captured_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, attempt_id) DO UPDATE SET
			image = excluded.image, container = excluded.container, exit_code = excluded.exit_code,
			oom_killed = excluded.oom_killed, reason = excluded.reason, message = excluded.message,
			started_at = excluded.started_at, finished_at = excluded.finished_at,
			log_tail = excluded.log_tail, captured_at = excluded.captured_at
	`, e.ServiceName, attemptID, e.Image, e.Container, e.ExitCode, e.OOMKilled, e.Reason, e.Message,
		formatOptionalTime(e.StartedAt), formatOptionalTime(e.FinishedAt), string(tail),
		captured.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: record deploy exit for %q: %w", e.ServiceName, err)
	}
	return nil
}

const deployExitColumns = `service_name, attempt_id, image, container, exit_code, oom_killed, reason, message,
	started_at, finished_at, log_tail, captured_at`

// GetDeployExit returns the capture for one attempt, or nil when it has none.
func (db *DB) GetDeployExit(ctx context.Context, serviceName, attemptID string) (*DeployExit, error) {
	row := db.QueryRowContext(ctx, `SELECT `+deployExitColumns+` FROM deploy_exit_captures
		WHERE service_name = ? AND attempt_id = ?`, serviceName, attemptID)
	return scanDeployExit(row.Scan)
}

// LatestDeployExit returns serviceName's most recent capture, or nil.
func (db *DB) LatestDeployExit(ctx context.Context, serviceName string) (*DeployExit, error) {
	row := db.QueryRowContext(ctx, `SELECT `+deployExitColumns+` FROM deploy_exit_captures
		WHERE service_name = ? ORDER BY captured_at DESC LIMIT 1`, serviceName)
	return scanDeployExit(row.Scan)
}

// DeleteDeployExits drops every capture of serviceName.
func (db *DB) DeleteDeployExits(ctx context.Context, serviceName string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_exit_captures WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete deploy exits for %q: %w", serviceName, err)
	}
	return nil
}

func scanDeployExit(scan func(...any) error) (*DeployExit, error) {
	var (
		e                           DeployExit
		started, finished, captured string
		tail                        string
	)
	err := scan(&e.ServiceName, &e.AttemptID, &e.Image, &e.Container, &e.ExitCode, &e.OOMKilled,
		&e.Reason, &e.Message, &started, &finished, &tail, &captured)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: scan deploy exit: %w", err)
	}
	e.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	e.FinishedAt, _ = time.Parse(time.RFC3339Nano, finished)
	e.CapturedAt, _ = time.Parse(time.RFC3339Nano, captured)
	if err := json.Unmarshal([]byte(tail), &e.LogTail); err != nil {
		return nil, fmt.Errorf("store: decode deploy exit log tail: %w", err)
	}
	return &e, nil
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
