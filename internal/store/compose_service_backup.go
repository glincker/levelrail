package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrComposeServiceBackupNotFound is returned when an (app, service) pair has no schedule row.
var ErrComposeServiceBackupNotFound = errors.New("store: compose service backup not found")

// ComposeServiceBackup is one compose database service's backup schedule.
type ComposeServiceBackup struct {
	App              string
	Service          string
	Engine           string
	BackupTargetID   string
	BackupSchedule   string
	BackupRetain     int
	BackupRetainDays int
	Enabled          bool
	// BackupPaused keeps the schedule configured but stops the scheduler from running it.
	BackupPaused bool
	CreatedAt    string
	UpdatedAt    string
}

// ComposeServiceBackupRun is one dump attempt of a compose database service.
type ComposeServiceBackupRun struct {
	ID         string
	App        string
	Service    string
	TargetID   string
	Status     string
	ObjectKey  string
	SizeBytes  int64
	Checksum   string
	Error      string
	StartedAt  string
	FinishedAt string
}

const composeServiceBackupCols = `app, service, engine, backup_target_id, backup_schedule, backup_retain, backup_retain_days, enabled, created_at, updated_at, backup_paused`

func scanComposeServiceBackup(s rowScanner) (ComposeServiceBackup, error) {
	var (
		c       ComposeServiceBackup
		target  sql.NullString
		enabled int
		paused  int
	)
	if err := s.Scan(&c.App, &c.Service, &c.Engine, &target, &c.BackupSchedule, &c.BackupRetain, &c.BackupRetainDays, &enabled, &c.CreatedAt, &c.UpdatedAt, &paused); err != nil {
		return ComposeServiceBackup{}, err
	}
	c.BackupTargetID = target.String
	c.Enabled = enabled != 0
	c.BackupPaused = paused != 0
	return c, nil
}

// UpsertComposeServiceBackup creates or replaces the schedule for c.App and c.Service, keeping created_at on update.
func (db *DB) UpsertComposeServiceBackup(ctx context.Context, c ComposeServiceBackup) error {
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := 0
	if c.Enabled {
		enabled = 1
	}
	paused := 0
	if c.BackupPaused {
		paused = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO compose_service_backups (`+composeServiceBackupCols+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (app, service) DO UPDATE SET
			engine = excluded.engine,
			backup_target_id = excluded.backup_target_id,
			backup_schedule = excluded.backup_schedule,
			backup_retain = excluded.backup_retain,
			backup_retain_days = excluded.backup_retain_days,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, c.App, c.Service, c.Engine, sql.NullString{String: c.BackupTargetID, Valid: c.BackupTargetID != ""},
		c.BackupSchedule, c.BackupRetain, c.BackupRetainDays, enabled, now, now, paused)
	if err != nil {
		return fmt.Errorf("store: upsert compose service backup %q/%q: %w", c.App, c.Service, err)
	}
	return nil
}

// GetComposeServiceBackup returns the schedule row, or ErrComposeServiceBackupNotFound.
func (db *DB) GetComposeServiceBackup(ctx context.Context, app, service string) (ComposeServiceBackup, error) {
	c, err := scanComposeServiceBackup(db.QueryRowContext(ctx,
		`SELECT `+composeServiceBackupCols+` FROM compose_service_backups WHERE app = ? AND service = ?`, app, service))
	if errors.Is(err, sql.ErrNoRows) {
		return ComposeServiceBackup{}, ErrComposeServiceBackupNotFound
	}
	if err != nil {
		return ComposeServiceBackup{}, fmt.Errorf("store: get compose service backup %q/%q: %w", app, service, err)
	}
	return c, nil
}

func (db *DB) queryComposeServiceBackups(ctx context.Context, where string, args ...any) ([]ComposeServiceBackup, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+composeServiceBackupCols+` FROM compose_service_backups `+where+` ORDER BY app, service`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list compose service backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ComposeServiceBackup
	for rows.Next() {
		c, err := scanComposeServiceBackup(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan compose service backup: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListComposeServiceBackups returns every schedule row of one app.
func (db *DB) ListComposeServiceBackups(ctx context.Context, app string) ([]ComposeServiceBackup, error) {
	return db.queryComposeServiceBackups(ctx, `WHERE app = ?`, app)
}

// ListScheduledComposeServices returns enabled rows that have both a cron schedule and a target.
func (db *DB) ListScheduledComposeServices(ctx context.Context) ([]ComposeServiceBackup, error) {
	return db.queryComposeServiceBackups(ctx, `WHERE backup_schedule != '' AND backup_target_id IS NOT NULL AND enabled = 1 AND backup_paused = 0`)
}

// SetComposeServiceBackupPaused pauses or resumes one service schedule; a service with no row is ErrComposeServiceBackupNotFound.
func (db *DB) SetComposeServiceBackupPaused(ctx context.Context, app, service string, paused bool) error {
	res, err := db.ExecContext(ctx, `UPDATE compose_service_backups SET backup_paused = ?, updated_at = ? WHERE app = ? AND service = ?`,
		paused, time.Now().UTC().Format(time.RFC3339), app, service)
	if err != nil {
		return fmt.Errorf("store: set compose service backup paused %q/%q: %w", app, service, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrComposeServiceBackupNotFound
	}
	return nil
}

// DeleteComposeServiceBackup removes the schedule row; run history is kept.
func (db *DB) DeleteComposeServiceBackup(ctx context.Context, app, service string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM compose_service_backups WHERE app = ? AND service = ?`, app, service)
	if err != nil {
		return fmt.Errorf("store: delete compose service backup %q/%q: %w", app, service, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrComposeServiceBackupNotFound
	}
	return nil
}

// DeleteComposeServiceBackupsForApp removes every schedule row of an app; run history is kept.
func (db *DB) DeleteComposeServiceBackupsForApp(ctx context.Context, app string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM compose_service_backups WHERE app = ?`, app); err != nil {
		return fmt.Errorf("store: delete compose service backups for app %q: %w", app, err)
	}
	return nil
}

// ComposeAppExists reports whether the app row or any of its member services is still stored.
// Compose services are stored as "<app>-<service>", so the app id is never a service name.
func (db *DB) ComposeAppExists(ctx context.Context, app string) (bool, error) {
	if _, err := db.GetApp(ctx, app); err == nil {
		return true, nil
	} else if !errors.Is(err, ErrAppNotFound) {
		return false, err
	}
	svcs, err := db.ListServicesByApp(ctx, app)
	if err != nil {
		return false, err
	}
	return len(svcs) > 0, nil
}

// StartComposeServiceBackupRun records a running attempt.
func (db *DB) StartComposeServiceBackupRun(ctx context.Context, r ComposeServiceBackupRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO compose_service_backup_runs (id, app, service, target_id, status, object_key, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.App, r.Service, r.TargetID, BackupStatusRunning, r.ObjectKey, r.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start compose service backup run %q: %w", r.ID, err)
	}
	return nil
}

// FinishComposeServiceBackupRun settles a running attempt as succeeded or failed.
func (db *DB) FinishComposeServiceBackupRun(ctx context.Context, id, status string, sizeBytes int64, checksum, errMsg, finishedAt string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE compose_service_backup_runs SET status = ?, size_bytes = ?, checksum = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, sizeBytes, checksum, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish compose service backup run %q: %w", id, err)
	}
	return nil
}

// ListComposeServiceBackupRuns returns the newest runs first, capped at limit.
func (db *DB) ListComposeServiceBackupRuns(ctx context.Context, app, service string, limit int) ([]ComposeServiceBackupRun, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, app, service, target_id, status, object_key, size_bytes, checksum, error, started_at, finished_at
		FROM compose_service_backup_runs WHERE app = ? AND service = ?
		ORDER BY started_at DESC, id DESC LIMIT ?
	`, app, service, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list compose service backup runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ComposeServiceBackupRun
	for rows.Next() {
		var r ComposeServiceBackupRun
		if err := rows.Scan(&r.ID, &r.App, &r.Service, &r.TargetID, &r.Status, &r.ObjectKey, &r.SizeBytes, &r.Checksum, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan compose service backup run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneComposeServiceBackupRuns deletes succeeded runs beyond the newest keep or older than olderThan,
// returning their objects so the caller can delete them. Running and failed rows are never pruned.
func (db *DB) PruneComposeServiceBackupRuns(ctx context.Context, app, service string, keep int, olderThan time.Time) ([]PrunedBackup, error) {
	if keep <= 0 && olderThan.IsZero() {
		return nil, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: prune compose service backup runs %q/%q: begin: %w", app, service, err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, target_id, object_key FROM compose_service_backup_runs
		WHERE app = ? AND service = ? AND status = ?
		AND (
			(? AND id NOT IN (
				SELECT id FROM compose_service_backup_runs
				WHERE app = ? AND service = ? AND status = ?
				ORDER BY started_at DESC LIMIT ?
			))
			OR (? AND started_at < ?)
		)
	`, app, service, BackupStatusSucceeded,
		keep > 0, app, service, BackupStatusSucceeded, keep,
		!olderThan.IsZero(), olderThan.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store: prune compose service backup runs %q/%q: select: %w", app, service, err)
	}
	var ids []string
	var pruned []PrunedBackup
	for rows.Next() {
		var id string
		var p PrunedBackup
		if err := rows.Scan(&id, &p.TargetID, &p.ObjectKey); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: prune compose service backup runs: scan: %w", err)
		}
		ids = append(ids, id)
		pruned = append(pruned, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM compose_service_backup_runs WHERE id = ?`, id); err != nil {
			return nil, fmt.Errorf("store: prune compose service backup runs: delete %q: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: prune compose service backup runs: commit: %w", err)
	}
	return pruned, nil
}

// CountScheduledComposeServiceBackups is how many compose services the scheduler has to watch.
func (db *DB) CountScheduledComposeServiceBackups(ctx context.Context) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM compose_service_backups WHERE backup_schedule != '' AND backup_target_id IS NOT NULL AND enabled = 1`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count scheduled compose service backups: %w", err)
	}
	return n, nil
}
