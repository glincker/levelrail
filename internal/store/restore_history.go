package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrRestoreHistoryNotFound is returned by FinishRestoreHistory when id
// doesn't match any row.
var ErrRestoreHistoryNotFound = errors.New("store: restore history record not found")

// RestoreHistory is one attempted restore of one database, or one app
// service's named volume, from one backup_history row. ResourceKind/
// ServiceName/VolumeName mirror BackupHistory's own identity fields
// exactly (backup_history.go), the same mutually-exclusive shape.
// Status reuses BackupStatusRunning/Succeeded/Failed (backup_history.go)
// rather than a second, identically-valued set of constants: a restore
// attempt moves through the exact same
// running-then-succeeded-or-failed lifecycle a backup attempt does, and
// migrations/0019_restore_history.sql's own CHECK constraint accepts the
// identical three strings, so two names for the same three values would
// only invite them drifting apart. Error is empty unless Status is
// BackupStatusFailed.
type RestoreHistory struct {
	ID              string
	DatabaseName    string
	ResourceKind    string
	ServiceName     string
	VolumeName      string
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
// h.ResourceKind defaults to BackupResourceKindDatabase when empty,
// mirroring StartBackupHistory's own identical default.
func (db *DB) StartRestoreHistory(ctx context.Context, h RestoreHistory) error {
	kind := h.ResourceKind
	if kind == "" {
		kind = BackupResourceKindDatabase
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO restore_history (id, database_name, resource_kind, service_name, volume_name, backup_history_id, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, '')
	`, h.ID, h.DatabaseName, kind, h.ServiceName, h.VolumeName, h.BackupHistoryID, BackupStatusRunning, h.StartedAt)
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
		SELECT id, database_name, resource_kind, service_name, volume_name, backup_history_id, status, error, started_at, finished_at
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

	out, err := scanRestoreHistoryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("store: list restore history for %q: %w", databaseName, err)
	}
	return out, nil
}

// scanRestoreHistoryRows reads every row of an already-executed
// restore_history query into RestoreHistory, the shared scan logic
// ListRestoreHistory and ListServiceVolumeRestoreHistory both need,
// mirroring scanBackupHistoryRows's identical reasoning
// (backup_history.go).
func scanRestoreHistoryRows(rows *sql.Rows) ([]RestoreHistory, error) {
	var out []RestoreHistory
	for rows.Next() {
		var h RestoreHistory
		if err := rows.Scan(&h.ID, &h.DatabaseName, &h.ResourceKind, &h.ServiceName, &h.VolumeName, &h.BackupHistoryID, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt); err != nil {
			return nil, fmt.Errorf("scan restore history row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate restore history rows: %w", err)
	}
	return out, nil
}

// ListServiceVolumeRestoreHistory returns every restore attempt for
// serviceName's volumeName, newest first, the volume counterpart of
// ListRestoreHistory.
func (db *DB) ListServiceVolumeRestoreHistory(ctx context.Context, serviceName, volumeName string) ([]RestoreHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, resource_kind, service_name, volume_name, backup_history_id, status, error, started_at, finished_at
		FROM restore_history
		WHERE resource_kind = ? AND service_name = ? AND volume_name = ?
		ORDER BY started_at DESC
	`, BackupResourceKindVolume, serviceName, volumeName)
	if err != nil {
		return nil, fmt.Errorf("store: list restore history for service %q volume %q: %w", serviceName, volumeName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out, err := scanRestoreHistoryRows(rows)
	if err != nil {
		return nil, fmt.Errorf("store: list restore history for service %q volume %q: %w", serviceName, volumeName, err)
	}
	return out, nil
}
