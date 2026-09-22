package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrBaseBackupHistoryNotFound is returned by FinishBaseBackupHistory and
// GetBaseBackupHistory when id doesn't match any row.
var ErrBaseBackupHistoryNotFound = errors.New("store: base backup history record not found")

// BaseBackupHistory is one attempt to take a physical base backup, the
// point-in-time-restore counterpart of BackupHistory (a logical dump).
// ObjectKey names a single pg_basebackup --format=tar object, which
// already embeds backup_label; no second object is needed. LSN is
// diagnostics-only; nothing in the restore path re-parses it.
type BaseBackupHistory struct {
	ID           string
	DatabaseName string
	TargetID     string
	ObjectKey    string
	LSN          string
	SizeBytes    int64
	Status       string
	Error        string
	StartedAt    string
	FinishedAt   string
}

// StartBaseBackupHistory records a base backup attempt beginning, status
// BackupStatusRunning, the same "write running before any real work
// happens" discipline StartBackupHistory's own doc comment explains for
// logical dumps.
func (db *DB) StartBaseBackupHistory(ctx context.Context, h BaseBackupHistory) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO base_backup_history (id, database_name, target_id, object_key, size_bytes, status, error, started_at, finished_at)
		VALUES (?, ?, ?, ?, 0, ?, '', ?, '')
	`, h.ID, h.DatabaseName, h.TargetID, h.ObjectKey, BackupStatusRunning, h.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start base backup history %q: %w", h.ID, err)
	}
	return nil
}

// FinishBaseBackupHistory updates a running base backup history row to
// its final status, recording the size and start LSN a succeeded attempt
// produced (both zero-value on a failed one, matching
// FinishBackupHistory's own convention for its checksum parameter).
func (db *DB) FinishBaseBackupHistory(ctx context.Context, id, status string, sizeBytes int64, lsn, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE base_backup_history
		SET status = ?, size_bytes = ?, lsn = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, sizeBytes, lsn, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish base backup history %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish base backup history %q: %w", id, err)
	}
	if n == 0 {
		return ErrBaseBackupHistoryNotFound
	}
	return nil
}

// GetBaseBackupHistory returns the base backup attempt with this ID, or
// ErrBaseBackupHistoryNotFound.
func (db *DB) GetBaseBackupHistory(ctx context.Context, id string) (BaseBackupHistory, error) {
	var h BaseBackupHistory
	err := db.QueryRowContext(ctx, `
		SELECT `+baseBackupHistoryColumns+`
		FROM base_backup_history
		WHERE id = ?
	`, id).Scan(baseBackupHistoryScanArgs(&h)...)
	if errors.Is(err, sql.ErrNoRows) {
		return BaseBackupHistory{}, ErrBaseBackupHistoryNotFound
	}
	if err != nil {
		return BaseBackupHistory{}, fmt.Errorf("store: get base backup history %q: %w", id, err)
	}
	return h, nil
}

// ListBaseBackupHistory returns every base backup attempt for
// databaseName, newest first: the frontend's "which base backups exist"
// view and, for a caller that needs every row rather than just the two
// boundary lookups below, the same full-history shape ListBackupHistory
// offers for logical dumps.
func (db *DB) ListBaseBackupHistory(ctx context.Context, databaseName string) ([]BaseBackupHistory, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+baseBackupHistoryColumns+`
		FROM base_backup_history
		WHERE database_name = ?
		ORDER BY started_at DESC
	`, databaseName)
	if err != nil {
		return nil, fmt.Errorf("store: list base backup history for %q: %w", databaseName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []BaseBackupHistory
	for rows.Next() {
		var h BaseBackupHistory
		if err := rows.Scan(baseBackupHistoryScanArgs(&h)...); err != nil {
			return nil, fmt.Errorf("store: scan base backup history row: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate base backup history rows: %w", err)
	}
	return out, nil
}

// GetOldestSucceededBaseBackup returns databaseName's earliest succeeded
// base backup, the lower bound of its PITR recoverable window: nothing
// before this moment can be replayed forward to, since no base backup
// exists to start from. ok is false, not an error, when there is no
// succeeded base backup yet (PITR just enabled, first base backup still
// running or never triggered).
func (db *DB) GetOldestSucceededBaseBackup(ctx context.Context, databaseName string) (h BaseBackupHistory, ok bool, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT `+baseBackupHistoryColumns+`
		FROM base_backup_history
		WHERE database_name = ? AND status = ?
		ORDER BY started_at ASC
		LIMIT 1
	`, databaseName, BackupStatusSucceeded).Scan(baseBackupHistoryScanArgs(&h)...)
	if errors.Is(err, sql.ErrNoRows) {
		return BaseBackupHistory{}, false, nil
	}
	if err != nil {
		return BaseBackupHistory{}, false, fmt.Errorf("store: get oldest succeeded base backup for %q: %w", databaseName, err)
	}
	return h, true, nil
}

// GetLatestBaseBackupBefore returns databaseName's most recent succeeded
// base backup with started_at <= cutoff: the one a PITR restore targeting
// cutoff must replay WAL forward from. ok is false when no succeeded base
// backup exists at or before cutoff, meaning cutoff is not reachable
// (either PITR was enabled after cutoff, or every base backup taken since
// is itself newer than cutoff).
func (db *DB) GetLatestBaseBackupBefore(ctx context.Context, databaseName string, cutoff time.Time) (h BaseBackupHistory, ok bool, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT `+baseBackupHistoryColumns+`
		FROM base_backup_history
		WHERE database_name = ? AND status = ? AND started_at <= ?
		ORDER BY started_at DESC
		LIMIT 1
	`, databaseName, BackupStatusSucceeded, cutoff.UTC().Format(time.RFC3339)).Scan(baseBackupHistoryScanArgs(&h)...)
	if errors.Is(err, sql.ErrNoRows) {
		return BaseBackupHistory{}, false, nil
	}
	if err != nil {
		return BaseBackupHistory{}, false, fmt.Errorf("store: get latest base backup before %s for %q: %w", cutoff, databaseName, err)
	}
	return h, true, nil
}

const baseBackupHistoryColumns = "id, database_name, target_id, object_key, lsn, size_bytes, status, error, started_at, finished_at"

// baseBackupHistoryScanArgs returns Scan destinations matching
// baseBackupHistoryColumns' order exactly, shared by every query above so
// that column list and scan order can never drift apart silently.
func baseBackupHistoryScanArgs(h *BaseBackupHistory) []any {
	return []any{&h.ID, &h.DatabaseName, &h.TargetID, &h.ObjectKey, &h.LSN, &h.SizeBytes, &h.Status, &h.Error, &h.StartedAt, &h.FinishedAt}
}
