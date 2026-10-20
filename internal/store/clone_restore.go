package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrCloneRestoreNotFound is returned by FinishCloneRestore when id
// doesn't match any row.
var ErrCloneRestoreNotFound = errors.New("store: clone restore record not found")

// CloneRestore is one attempted "restore as new database": a backup of
// SourceDatabaseName restored into NewDatabaseName, a database created
// fresh for this attempt rather than overwriting anything. Status reuses
// BackupStatusRunning/Succeeded/Failed, the same reasoning
// RestoreHistory's own doc comment gives for its identical lifecycle.
type CloneRestore struct {
	ID                 string
	SourceDatabaseName string
	NewDatabaseName    string
	BackupHistoryID    string
	Status             string
	Error              string
	StartedAt          string
	FinishedAt         string
}

// StartCloneRestore records a clone-restore attempt beginning, status
// BackupStatusRunning, before the new database has even become reachable:
// the same "write running eagerly" reasoning StartRestoreHistory's own
// doc comment gives.
func (db *DB) StartCloneRestore(ctx context.Context, h CloneRestore) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO clone_restores (id, source_database_name, new_database_name, backup_history_id, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, '', ?, '')
	`, h.ID, h.SourceDatabaseName, h.NewDatabaseName, h.BackupHistoryID, BackupStatusRunning, h.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start clone restore %q: %w", h.ID, err)
	}
	return nil
}

// FinishCloneRestore updates a running clone-restore row to its final
// status, mirroring FinishRestoreHistory exactly.
func (db *DB) FinishCloneRestore(ctx context.Context, id, status, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE clone_restores
		SET status = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish clone restore %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish clone restore %q: %w", id, err)
	}
	if n == 0 {
		return ErrCloneRestoreNotFound
	}
	return nil
}

// ListCloneRestores returns every clone-restore attempt sourced from
// sourceDatabaseName, newest first: the operator triggers this action
// from the source database's own backups view (RestoreBackupDialog's
// "restore as new database" counterpart), so that's the view this list
// serves, the same way ListRestoreHistory serves the in-place restore
// table on that same page.
func (db *DB) ListCloneRestores(ctx context.Context, sourceDatabaseName string) ([]CloneRestore, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, source_database_name, new_database_name, backup_history_id, status, error, started_at, finished_at
		FROM clone_restores
		WHERE source_database_name = ?
		ORDER BY started_at DESC
	`, sourceDatabaseName)
	if err != nil {
		return nil, fmt.Errorf("store: list clone restores for %q: %w", sourceDatabaseName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []CloneRestore
	for rows.Next() {
		var h CloneRestore
		if err := rows.Scan(&h.ID, &h.SourceDatabaseName, &h.NewDatabaseName, &h.BackupHistoryID, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan clone restore row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate clone restore rows: %w", err)
	}
	return out, nil
}
