package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Backup attempt statuses migrations/0018_backup_targets.sql's CHECK
// constraint accepts. "running" is written first (StartBackupHistory)
// so a backup that crashes the control plane mid-upload leaves a real,
// visible "running" row behind rather than no record at all; nothing
// currently sweeps a stuck "running" row back to "failed", a known,
// honest gap for the eventual scheduler to close alongside its own
// retry logic, not invented speculatively here.
const (
	BackupStatusRunning   = "running"
	BackupStatusSucceeded = "succeeded"
	BackupStatusFailed    = "failed"
)

// ErrBackupHistoryNotFound is returned by FinishBackupHistory when id
// doesn't match any row.
var ErrBackupHistoryNotFound = errors.New("store: backup history record not found")

// BackupHistory is one attempted backup of one database to one backup
// target. Error is empty unless Status is BackupStatusFailed.
type BackupHistory struct {
	ID           string
	DatabaseName string
	TargetID     string
	ObjectKey    string
	SizeBytes    int64
	Status       string
	Error        string
	StartedAt    string
	FinishedAt   string
}

// StartBackupHistory records a backup attempt beginning, status
// BackupStatusRunning, before the dump or upload has done any real
// work: see this file's own status-constants comment for why "running"
// is written eagerly rather than only recording success/failure after
// the fact.
func (db *DB) StartBackupHistory(ctx context.Context, h BackupHistory) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_history (id, database_name, target_id, object_key, size_bytes, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, 0, ?, '', ?, '')
	`, h.ID, h.DatabaseName, h.TargetID, h.ObjectKey, BackupStatusRunning, h.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start backup history %q: %w", h.ID, err)
	}
	return nil
}

// FinishBackupHistory updates a running backup history row to its final
// status. sizeBytes and errMsg are both ignored (left as-is / not
// applicable) when status is BackupStatusRunning, which callers should
// never actually pass here since StartBackupHistory already wrote that
// state; FinishBackupHistory exists specifically for the succeeded/failed
// transition.
func (db *DB) FinishBackupHistory(ctx context.Context, id, status string, sizeBytes int64, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE backup_history
		SET status = ?, size_bytes = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, sizeBytes, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish backup history %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish backup history %q: %w", id, err)
	}
	if n == 0 {
		return ErrBackupHistoryNotFound
	}
	return nil
}

// GetBackupHistory returns the backup attempt with this ID, or
// ErrBackupHistoryNotFound. Added for the restore path
// (internal/backup.RestoreRunner): a restore is always initiated by
// naming a specific past backup attempt, so unlike the forward backup
// path (which only ever lists history, never looks up one row by ID),
// restoring needs to resolve exactly one row, including its Status,
// TargetID, and ObjectKey, before anything else can happen.
func (db *DB) GetBackupHistory(ctx context.Context, id string) (BackupHistory, error) {
	var h BackupHistory
	err := db.QueryRowContext(ctx, `
		SELECT id, database_name, target_id, object_key, size_bytes, status, error, started_at, finished_at
		FROM backup_history
		WHERE id = ?
	`, id).Scan(&h.ID, &h.DatabaseName, &h.TargetID, &h.ObjectKey, &h.SizeBytes, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupHistory{}, ErrBackupHistoryNotFound
	}
	if err != nil {
		return BackupHistory{}, fmt.Errorf("store: get backup history %q: %w", id, err)
	}
	return h, nil
}

// PruneBackupHistory deletes every succeeded backup_history row for
// databaseName beyond the newest keep, ordered by started_at, and
// returns how many rows were removed. Only BackupStatusSucceeded rows
// are ever eligible: a running or failed attempt carries diagnostic
// value a retention count was never meant to bound (an operator who
// sets retain: 7 wants "7 successful backups," not "7 attempts of any
// kind"), so those are left untouched regardless of how old they are.
//
// keep <= 0 deletes nothing and returns (0, nil) without issuing a
// query: this mirrors store.DesiredDatabase.BackupRetain's own "0 means
// keep everything" meaning (see migrations/0023's own doc comment) at
// the one call site that matters, internal/backup.Scheduler, rather than
// making every caller remember to guard the zero case itself.
//
// This is scoped narrowly to what internal/backup.Scheduler's own
// retention step needs after a successful scheduled run: it prunes
// store rows only, never the underlying object an Uploader wrote to a
// backup_targets bucket, which is a real, known, and deliberately
// undone gap here (deleting that object needs live target credentials
// resolved through internal/secrets plus a Deleter capability
// internal/backup does not have today, see that package's own Uploader
// doc comment) left for a dedicated retention/pruning feature to close
// properly, not invented speculatively as a side effect of this one.
func (db *DB) PruneBackupHistory(ctx context.Context, databaseName string, keep int) (int64, error) {
	if keep <= 0 {
		return 0, nil
	}

	res, err := db.ExecContext(ctx, `
		DELETE FROM backup_history
		WHERE database_name = ? AND status = ? AND id NOT IN (
			SELECT id FROM backup_history
			WHERE database_name = ? AND status = ?
			ORDER BY started_at DESC
			LIMIT ?
		)
	`, databaseName, BackupStatusSucceeded, databaseName, BackupStatusSucceeded, keep)
	if err != nil {
		return 0, fmt.Errorf("store: prune backup history for %q: %w", databaseName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: prune backup history for %q: rows affected: %w", databaseName, err)
	}
	return n, nil
}

// ListBackupHistory returns every backup attempt for databaseName,
// newest first (migrations/0018's own index is built for exactly this
// query shape).
func (db *DB) ListBackupHistory(ctx context.Context, databaseName string) ([]BackupHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, target_id, object_key, size_bytes, status, error, started_at, finished_at
		FROM backup_history
		WHERE database_name = ?
		ORDER BY started_at DESC
	`, databaseName)
	if err != nil {
		return nil, fmt.Errorf("store: list backup history for %q: %w", databaseName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []BackupHistory
	for rows.Next() {
		var h BackupHistory
		if err := rows.Scan(&h.ID, &h.DatabaseName, &h.TargetID, &h.ObjectKey, &h.SizeBytes, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan backup history row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate backup history rows: %w", err)
	}
	return out, nil
}
