package store

import (
	"context"
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
