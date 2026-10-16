package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Error issue statuses.
const (
	ErrorStatusOpen     = "open"
	ErrorStatusResolved = "resolved"
	ErrorStatusIgnored  = "ignored"
)

// DefaultErrorEventsPerIssue is how many sample events one issue keeps.
const DefaultErrorEventsPerIssue = 20

// ErrNoErrorIssue is returned when an issue does not exist for the app.
var ErrNoErrorIssue = errors.New("telemetry: error issue not found")

// ErrorEvent is one received error, already grouped by Fingerprint.
type ErrorEvent struct {
	ResourceID  string
	Fingerprint string
	Title       string
	Level       string
	EventID     string
	TimestampNs int64
	Release     string
	Environment string
	DeployID    string
	Raw         string
}

// ErrorIssue is one group of events as the list shows it.
type ErrorIssue struct {
	Fingerprint  string `json:"fingerprint"`
	Title        string `json:"title"`
	Level        string `json:"level"`
	FirstSeenNs  int64  `json:"first_seen_ns"`
	LastSeenNs   int64  `json:"last_seen_ns"`
	Count        int64  `json:"count"`
	LastRelease  string `json:"last_release,omitempty"`
	LastDeployID string `json:"last_deploy_id,omitempty"`
	Status       string `json:"status"`
	Regressed    bool   `json:"regressed"`
}

// ErrorSample is one stored event of an issue.
type ErrorSample struct {
	EventID     string `json:"event_id"`
	TimestampNs int64  `json:"timestamp_ns"`
	Release     string `json:"release,omitempty"`
	Environment string `json:"environment,omitempty"`
	DeployID    string `json:"deploy_id,omitempty"`
	Raw         string `json:"-"`
}

// ErrorFilter narrows ListErrorIssues.
type ErrorFilter struct {
	ResourceID string
	Status     string
	Since      time.Time
	Limit      int
}

// WriteErrorEvent records ev against its issue, creating the issue or bumping
// its count, and trims the issue's samples to keep. A resolved issue that
// receives an event reopens flagged as a regression, reported as regressed.
func (db *DB) WriteErrorEvent(ctx context.Context, ev ErrorEvent, keep int) (regressed bool, err error) {
	if keep <= 0 {
		keep = DefaultErrorEventsPerIssue
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("telemetry: begin error write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM error_issues WHERE resource_id = ? AND fingerprint = ?`,
		ev.ResourceID, ev.Fingerprint).Scan(&status)
	deploy := nullString(ev.DeployID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = tx.ExecContext(ctx, `INSERT INTO error_issues
			(resource_id, fingerprint, title, level, first_seen_ns, last_seen_ns, count, last_release, last_deploy_id, status)
			VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, 'open')`,
			ev.ResourceID, ev.Fingerprint, ev.Title, ev.Level, ev.TimestampNs, ev.TimestampNs, ev.Release, deploy)
	case err != nil:
		return false, fmt.Errorf("telemetry: load error issue: %w", err)
	default:
		regressed = status == ErrorStatusResolved
		_, err = tx.ExecContext(ctx, `UPDATE error_issues SET count = count + 1,
			last_seen_ns = MAX(last_seen_ns, ?), level = ?, last_release = ?, last_deploy_id = ?,
			status = CASE WHEN status = 'resolved' THEN 'open' ELSE status END,
			regressed = CASE WHEN status = 'resolved' THEN 1 ELSE regressed END
			WHERE resource_id = ? AND fingerprint = ?`,
			ev.TimestampNs, ev.Level, ev.Release, deploy, ev.ResourceID, ev.Fingerprint)
	}
	if err != nil {
		return false, fmt.Errorf("telemetry: write error issue: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO error_events
		(resource_id, fingerprint, event_id, ts_ns, release, environment, deploy_id, raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ResourceID, ev.Fingerprint, ev.EventID, ev.TimestampNs, ev.Release, ev.Environment, deploy, ev.Raw); err != nil {
		return false, fmt.Errorf("telemetry: write error event: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM error_events WHERE resource_id = ? AND fingerprint = ? AND id NOT IN (
		SELECT id FROM error_events WHERE resource_id = ? AND fingerprint = ? ORDER BY id DESC LIMIT ?)`,
		ev.ResourceID, ev.Fingerprint, ev.ResourceID, ev.Fingerprint, keep); err != nil {
		return false, fmt.Errorf("telemetry: trim error events: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("telemetry: commit error write: %w", err)
	}
	return regressed, nil
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

const errorIssueCols = `fingerprint, title, level, first_seen_ns, last_seen_ns, count, last_release, last_deploy_id, status, regressed`

func scanErrorIssue(sc interface{ Scan(...any) error }) (ErrorIssue, error) {
	var i ErrorIssue
	var deploy sql.NullString
	var reg int
	if err := sc.Scan(&i.Fingerprint, &i.Title, &i.Level, &i.FirstSeenNs, &i.LastSeenNs, &i.Count,
		&i.LastRelease, &deploy, &i.Status, &reg); err != nil {
		return ErrorIssue{}, err
	}
	i.LastDeployID = deploy.String
	i.Regressed = reg != 0
	return i, nil
}

// ListErrorIssues returns the app's issues, most recently seen first.
func (db *DB) ListErrorIssues(ctx context.Context, f ErrorFilter) ([]ErrorIssue, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	var since int64
	if !f.Since.IsZero() {
		since = f.Since.UnixNano()
	}
	rows, err := db.QueryContext(ctx, `SELECT `+errorIssueCols+` FROM error_issues
		WHERE resource_id = ? AND last_seen_ns >= ? AND (? = '' OR status = ?)
		ORDER BY last_seen_ns DESC LIMIT ?`, f.ResourceID, since, f.Status, f.Status, f.Limit)
	if err != nil {
		return nil, fmt.Errorf("telemetry: list error issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ErrorIssue
	for rows.Next() {
		i, err := scanErrorIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("telemetry: scan error issue: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// GetErrorIssue returns one issue and its stored samples, newest first.
func (db *DB) GetErrorIssue(ctx context.Context, resourceID, fingerprint string) (ErrorIssue, []ErrorSample, error) {
	i, err := scanErrorIssue(db.QueryRowContext(ctx, `SELECT `+errorIssueCols+` FROM error_issues
		WHERE resource_id = ? AND fingerprint = ?`, resourceID, fingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrorIssue{}, nil, ErrNoErrorIssue
	}
	if err != nil {
		return ErrorIssue{}, nil, fmt.Errorf("telemetry: get error issue: %w", err)
	}
	rows, err := db.QueryContext(ctx, `SELECT event_id, ts_ns, release, environment, deploy_id, raw FROM error_events
		WHERE resource_id = ? AND fingerprint = ? ORDER BY id DESC`, resourceID, fingerprint)
	if err != nil {
		return ErrorIssue{}, nil, fmt.Errorf("telemetry: list error events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ErrorSample
	for rows.Next() {
		var s ErrorSample
		var deploy sql.NullString
		if err := rows.Scan(&s.EventID, &s.TimestampNs, &s.Release, &s.Environment, &deploy, &s.Raw); err != nil {
			return ErrorIssue{}, nil, fmt.Errorf("telemetry: scan error event: %w", err)
		}
		s.DeployID = deploy.String
		out = append(out, s)
	}
	return i, out, rows.Err()
}

// SetErrorIssueStatus changes an issue's status and clears its regression flag.
func (db *DB) SetErrorIssueStatus(ctx context.Context, resourceID, fingerprint, status string) error {
	res, err := db.ExecContext(ctx, `UPDATE error_issues SET status = ?, regressed = 0
		WHERE resource_id = ? AND fingerprint = ?`, status, resourceID, fingerprint)
	if err != nil {
		return fmt.Errorf("telemetry: set error issue status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoErrorIssue
	}
	return nil
}

// RetainErrors deletes events and issues last seen before cutoff.
func (db *DB) RetainErrors(ctx context.Context, cutoff time.Time) (int64, error) {
	ns := cutoff.UnixNano()
	if _, err := db.ExecContext(ctx, `DELETE FROM error_events WHERE ts_ns < ?`, ns); err != nil {
		return 0, fmt.Errorf("telemetry: error event retention sweep: %w", err)
	}
	res, err := db.ExecContext(ctx, `DELETE FROM error_issues WHERE last_seen_ns < ?`, ns)
	if err != nil {
		return 0, fmt.Errorf("telemetry: error issue retention sweep: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("telemetry: error issue retention rows: %w", err)
	}
	return n, nil
}
