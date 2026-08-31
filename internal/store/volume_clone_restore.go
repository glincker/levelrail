package store

import (
	"context"
	"errors"
	"fmt"
)

// ErrVolumeCloneRestoreNotFound is returned by FinishVolumeCloneRestore
// when id doesn't match any row.
var ErrVolumeCloneRestoreNotFound = errors.New("store: volume clone restore record not found")

// VolumeCloneRestore is one attempted "restore as new volume": a backup of
// SourceServiceName's SourceVolumeName restored into NewVolumeName, a
// brand-new Docker volume created fresh for this attempt rather than
// overwriting anything. The app service volume counterpart of
// CloneRestore, with the same status lifecycle.
type VolumeCloneRestore struct {
	ID                string
	SourceServiceName string
	SourceVolumeName  string
	NewVolumeName     string
	BackupHistoryID   string
	Status            string
	Error             string
	StartedAt         string
	FinishedAt        string
}

// StartVolumeCloneRestore records a clone-restore attempt beginning,
// status BackupStatusRunning, mirroring StartCloneRestore exactly.
func (db *DB) StartVolumeCloneRestore(ctx context.Context, h VolumeCloneRestore) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO volume_clone_restores (id, source_service_name, source_volume_name, new_volume_name, backup_history_id, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, '')
	`, h.ID, h.SourceServiceName, h.SourceVolumeName, h.NewVolumeName, h.BackupHistoryID, BackupStatusRunning, h.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start volume clone restore %q: %w", h.ID, err)
	}
	return nil
}

// FinishVolumeCloneRestore updates a running clone-restore row to its
// final status, mirroring FinishCloneRestore exactly.
func (db *DB) FinishVolumeCloneRestore(ctx context.Context, id, status, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE volume_clone_restores
		SET status = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish volume clone restore %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish volume clone restore %q: %w", id, err)
	}
	if n == 0 {
		return ErrVolumeCloneRestoreNotFound
	}
	return nil
}

// ListVolumeCloneRestores returns every clone-restore attempt sourced from
// sourceServiceName/sourceVolumeName, newest first, mirroring
// ListCloneRestores exactly.
func (db *DB) ListVolumeCloneRestores(ctx context.Context, sourceServiceName, sourceVolumeName string) ([]VolumeCloneRestore, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, source_service_name, source_volume_name, new_volume_name, backup_history_id, status, error, started_at, finished_at
		FROM volume_clone_restores
		WHERE source_service_name = ? AND source_volume_name = ?
		ORDER BY started_at DESC
	`, sourceServiceName, sourceVolumeName)
	if err != nil {
		return nil, fmt.Errorf("store: list volume clone restores for %q/%q: %w", sourceServiceName, sourceVolumeName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []VolumeCloneRestore
	for rows.Next() {
		var h VolumeCloneRestore
		if err := rows.Scan(&h.ID, &h.SourceServiceName, &h.SourceVolumeName, &h.NewVolumeName, &h.BackupHistoryID, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt); err != nil {
			return nil, fmt.Errorf("store: scan volume clone restore row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate volume clone restore rows: %w", err)
	}
	return out, nil
}
