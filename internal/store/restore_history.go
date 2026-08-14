package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrRestoreHistoryNotFound is returned by FinishRestoreHistory when id
// doesn't match any row.
var ErrRestoreHistoryNotFound = errors.New("store: restore history record not found")

// RestoreHistory is one attempted restore of one database from one
// backup_history row. Status reuses BackupStatusRunning/Succeeded/Failed
// (backup_history.go) rather than a second, identically-valued set of
// constants: a restore attempt moves through the exact same
// running-then-succeeded-or-failed lifecycle a backup attempt does, and
// migrations/0019_restore_history.sql's own CHECK constraint accepts the
// identical three strings, so two names for the same three values would
// only invite them drifting apart. Error is empty unless Status is
// BackupStatusFailed.
type RestoreHistory struct {
	ID              string
	DatabaseName    string
	BackupHistoryID string
	Status          string
	Error           string
	StartedAt       string
	FinishedAt      string
}

// StartRestoreHistory records a restore attempt beginning, status
// BackupStatusRunning, before the download or restore has done any real
// work: the same "write running eagerly so a mid-attempt crash leaves a
// real row behind" reasoning StartBackupHistory's own doc comment gives.
func (db *DB) StartRestoreHistory(ctx context.Context, h RestoreHistory) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO restore_history (id, database_name, backup_history_id, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, '', ?, '')
	`, h.ID, h.DatabaseName, h.BackupHistoryID, BackupStatusRunning, h.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start restore history %q: %w", h.ID, err)
	}
	return nil
}

// FinishRestoreHistory updates a running restore history row to its
// final status, the restore counterpart of FinishBackupHistory (no
// sizeBytes parameter: nothing is uploaded on a restore, so there is no
// equivalent byte count to record).
func (db *DB) FinishRestoreHistory(ctx context.Context, id, status, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE restore_history
		SET status = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish restore history %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish restore history %q: %w", id, err)
	}
	if n == 0 {
		return ErrRestoreHistoryNotFound
	}
	return nil
}

// ListRestoreHistory returns every restore attempt for databaseName,
// newest first (migrations/0019's own index is built for exactly this
// query shape, the same as ListBackupHistory's).
func (db *DB) ListRestoreHistory(ctx context.Context, databaseName string) ([]RestoreHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, backup_history_id, status, error, started_at, finished_at
		FROM restore_history
		WHERE database_name = ?
		ORDER BY started_at DESC
	`, databaseName)
	if err != nil {
		return nil, fmt.Errorf("store: list restore history for %q: %w", databaseName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []RestoreHistory
	for rows.Next() {
		var h RestoreHistory
		if err := rows.Scan(&h.ID, &h.DatabaseName, &h.BackupHistoryID, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan restore history row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate restore history rows: %w", err)
	}
	return out, nil
}
