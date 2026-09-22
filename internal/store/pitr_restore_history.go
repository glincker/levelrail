package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrPITRRestoreHistoryNotFound is returned by FinishPITRRestoreHistory
// when id doesn't match any row.
var ErrPITRRestoreHistoryNotFound = errors.New("store: pitr restore history record not found")

// PITRRestoreHistory is one attempted point-in-time restore of one
// database, restore_history's PITR counterpart (see
// migrations/0115_database_pitr.sql's own comment for why this is a
// separate table rather than new columns on restore_history).
type PITRRestoreHistory struct {
	ID                  string
	DatabaseName        string
	BaseBackupHistoryID string
	TargetTimestamp     string
	Status              string
	Error               string
	StartedAt           string
	FinishedAt          string
}

// StartPITRRestoreHistory records a PITR restore attempt beginning,
// status BackupStatusRunning, the same "write running before any real
// work happens" discipline StartRestoreHistory's own doc comment
// explains.
func (db *DB) StartPITRRestoreHistory(ctx context.Context, h PITRRestoreHistory) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO pitr_restore_history (id, database_name, base_backup_history_id, target_timestamp, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, '', ?, '')
	`, h.ID, h.DatabaseName, h.BaseBackupHistoryID, h.TargetTimestamp, BackupStatusRunning, h.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start pitr restore history %q: %w", h.ID, err)
	}
	return nil
}

// FinishPITRRestoreHistory updates a running PITR restore history row to
// its final status.
func (db *DB) FinishPITRRestoreHistory(ctx context.Context, id, status, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE pitr_restore_history
		SET status = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish pitr restore history %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish pitr restore history %q: %w", id, err)
	}
	if n == 0 {
		return ErrPITRRestoreHistoryNotFound
	}
	return nil
}

// ListPITRRestoreHistory returns every PITR restore attempt for
// databaseName, newest first.
func (db *DB) ListPITRRestoreHistory(ctx context.Context, databaseName string) ([]PITRRestoreHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, base_backup_history_id, target_timestamp, status, error, started_at, finished_at
		FROM pitr_restore_history
		WHERE database_name = ?
		ORDER BY started_at DESC
	`, databaseName)
	if err != nil {
		return nil, fmt.Errorf("store: list pitr restore history for %q: %w", databaseName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PITRRestoreHistory
	for rows.Next() {
		var h PITRRestoreHistory
		if err := rows.Scan(&h.ID, &h.DatabaseName, &h.BaseBackupHistoryID, &h.TargetTimestamp, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan pitr restore history row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate pitr restore history rows: %w", err)
	}
	return out, nil
}

// GetPITRRestoreHistory returns the PITR restore attempt with this ID,
// or ErrPITRRestoreHistoryNotFound.
func (db *DB) GetPITRRestoreHistory(ctx context.Context, id string) (PITRRestoreHistory, error) {
	var h PITRRestoreHistory
	err := db.QueryRowContext(ctx, `
		SELECT id, database_name, base_backup_history_id, target_timestamp, status, error, started_at, finished_at
		FROM pitr_restore_history
		WHERE id = ?
	`, id).Scan(&h.ID, &h.DatabaseName, &h.BaseBackupHistoryID, &h.TargetTimestamp, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PITRRestoreHistory{}, ErrPITRRestoreHistoryNotFound
	}
	if err != nil {
		return PITRRestoreHistory{}, fmt.Errorf("store: get pitr restore history %q: %w", id, err)
	}
	return h, nil
}
