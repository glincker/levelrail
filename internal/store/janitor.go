package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JanitorRun is one cleanup pass as recorded in history.
type JanitorRun struct {
	ID             string
	StartedAt      time.Time
	Trigger        string
	DryRun         bool
	ReclaimedBytes int64
	FreePercent    float64
	Body           string
	Error          string
}

// JanitorQuarantine is a volume held back before deletion.
type JanitorQuarantine struct {
	Kind      string
	Name      string
	SizeBytes int64
	FirstSeen time.Time
}

// PrunableAttempt is a finished deploy attempt that retention may delete.
type PrunableAttempt struct {
	ID          string
	ServiceName string
	Status      string
	StartedAt   time.Time
}

// GetJanitorPolicy returns the stored policy JSON, ok false when never saved.
func (db *DB) GetJanitorPolicy(ctx context.Context) (string, bool, error) {
	var body string
	err := db.QueryRowContext(ctx, `SELECT body FROM janitor_policy WHERE id = 1`).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get janitor policy: %w", err)
	}
	return body, true, nil
}

// SetJanitorPolicy stores the policy JSON.
func (db *DB) SetJanitorPolicy(ctx context.Context, body, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO janitor_policy (id, body, updated_at, updated_by) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body = excluded.body, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		body, now.UTC().Format(time.RFC3339Nano), actor)
	if err != nil {
		return fmt.Errorf("store: set janitor policy: %w", err)
	}
	return nil
}

// InsertJanitorRun appends a run to history and trims history to keep rows.
func (db *DB) InsertJanitorRun(ctx context.Context, r JanitorRun, keep int) error {
	dry := 0
	if r.DryRun {
		dry = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO janitor_runs (id, started_at, trigger, dry_run, reclaimed_bytes, free_percent, body, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.StartedAt.UTC().Format(time.RFC3339Nano), r.Trigger, dry, r.ReclaimedBytes, r.FreePercent, r.Body, r.Error)
	if err != nil {
		return fmt.Errorf("store: insert janitor run: %w", err)
	}
	if keep > 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM janitor_runs WHERE id NOT IN
			(SELECT id FROM janitor_runs ORDER BY started_at DESC LIMIT ?)`, keep); err != nil {
			return fmt.Errorf("store: trim janitor runs: %w", err)
		}
	}
	return nil
}

// ListJanitorRuns returns the newest runs first.
func (db *DB) ListJanitorRuns(ctx context.Context, limit int) ([]JanitorRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, started_at, trigger, dry_run, reclaimed_bytes, free_percent, body, error
		FROM janitor_runs ORDER BY started_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list janitor runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []JanitorRun
	for rows.Next() {
		var r JanitorRun
		var started string
		var dry int
		if err := rows.Scan(&r.ID, &started, &r.Trigger, &dry, &r.ReclaimedBytes, &r.FreePercent, &r.Body, &r.Error); err != nil {
			return nil, fmt.Errorf("store: scan janitor run: %w", err)
		}
		r.DryRun = dry == 1
		if r.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, fmt.Errorf("store: parse janitor run time: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertJanitorQuarantine records first sight of a candidate, keeping the original first_seen.
func (db *DB) UpsertJanitorQuarantine(ctx context.Context, q JanitorQuarantine) error {
	_, err := db.ExecContext(ctx, `INSERT INTO janitor_quarantine (kind, name, size_bytes, first_seen) VALUES (?, ?, ?, ?)
		ON CONFLICT(kind, name) DO UPDATE SET size_bytes = excluded.size_bytes`,
		q.Kind, q.Name, q.SizeBytes, q.FirstSeen.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: upsert janitor quarantine: %w", err)
	}
	return nil
}

// ListJanitorQuarantine returns every quarantined item of a kind.
func (db *DB) ListJanitorQuarantine(ctx context.Context, kind string) ([]JanitorQuarantine, error) {
	rows, err := db.QueryContext(ctx, `SELECT kind, name, size_bytes, first_seen FROM janitor_quarantine WHERE kind = ? ORDER BY first_seen`, kind)
	if err != nil {
		return nil, fmt.Errorf("store: list janitor quarantine: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []JanitorQuarantine
	for rows.Next() {
		var q JanitorQuarantine
		var seen string
		if err := rows.Scan(&q.Kind, &q.Name, &q.SizeBytes, &seen); err != nil {
			return nil, fmt.Errorf("store: scan janitor quarantine: %w", err)
		}
		if q.FirstSeen, err = time.Parse(time.RFC3339Nano, seen); err != nil {
			return nil, fmt.Errorf("store: parse janitor quarantine time: %w", err)
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// DeleteJanitorQuarantine releases an item from quarantine.
func (db *DB) DeleteJanitorQuarantine(ctx context.Context, kind, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM janitor_quarantine WHERE kind = ? AND name = ?`, kind, name); err != nil {
		return fmt.Errorf("store: delete janitor quarantine: %w", err)
	}
	return nil
}

// ListPrunableDeployAttempts returns failed, superseded, canceled and held
// attempts started before cutoff, skipping each service's newest keep attempts
// and any attempt a rollback pin references. Succeeded attempts never match.
func (db *DB) ListPrunableDeployAttempts(ctx context.Context, cutoff time.Time, keep, limit int) ([]PrunableAttempt, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, service_name, status, started_at FROM (
			SELECT id, service_name, status, started_at,
				ROW_NUMBER() OVER (PARTITION BY service_name ORDER BY started_at DESC) AS rn
			FROM deploy_attempts)
		WHERE rn > ? AND started_at < ?
			AND status IN (?, ?, ?, ?)
			AND id NOT IN (SELECT deploy_attempt_id FROM rollback_pins)
		ORDER BY started_at LIMIT ?`,
		keep, cutoff.UTC().Format(time.RFC3339Nano),
		DeployAttemptStatusFailed, DeployAttemptStatusSuperseded, DeployAttemptStatusCanceled, DeployAttemptStatusHeld, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list prunable deploy attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PrunableAttempt
	for rows.Next() {
		var a PrunableAttempt
		var started string
		if err := rows.Scan(&a.ID, &a.ServiceName, &a.Status, &started); err != nil {
			return nil, fmt.Errorf("store: scan prunable deploy attempt: %w", err)
		}
		if a.StartedAt, err = time.Parse(time.RFC3339Nano, started); err != nil {
			return nil, fmt.Errorf("store: parse prunable attempt time: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

var attemptDependents = []struct{ table, column string }{
	{"probe_attempts", "deploy_attempt_id"},
	{"deploy_supply_chain", "attempt_id"},
	{"secret_scan_runs", "attempt_id"},
	{"deploy_urls", "attempt_id"},
	{"deploy_gate_events", "deploy_attempt_id"},
	{"deploy_exit_captures", "attempt_id"},
	{"deploy_trace_steps", "attempt_id"},
	{"deploy_timelines", "attempt_id"},
	{"deploy_attempts", "id"},
}

// DeleteDeployAttempts removes the attempts and the rows that describe only them.
func (db *DB) DeleteDeployAttempts(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete deploy attempts: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, d := range attemptDependents {
		q := "DELETE FROM " + d.table + " WHERE " + d.column + " IN (" + marks + ")" //nolint:gosec // table and column come from the fixed list above
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("store: delete %s rows: %w", d.table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: delete deploy attempts: %w", err)
	}
	return nil
}
