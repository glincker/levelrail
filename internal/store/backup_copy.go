package store

import (
	"context"
	"errors"
	"fmt"
)

// Backup copy statuses accepted by migrations/3047_backup_copies.sql.
const (
	BackupCopyPending   = "pending"
	BackupCopyCopying   = "copying"
	BackupCopySucceeded = "succeeded"
	BackupCopyFailed    = "failed"
)

// ErrBackupCopyTargetInvalid means the chosen second destination is missing or is the primary of every backup.
var ErrBackupCopyTargetInvalid = errors.New("store: backup copy target is not set")

// BackupCopySettings is the single second-destination setting.
type BackupCopySettings struct {
	Enabled   bool
	TargetID  string
	EnabledAt string
}

// BackupCopy is one backup's copy to the second destination, joined with the backup it mirrors.
type BackupCopy struct {
	BackupHistoryID string
	ResourceName    string
	ResourceKind    string
	SourceTargetID  string
	ObjectKey       string
	BackupSize      int64
	BackupFinished  string
	TargetID        string
	Status          string
	Attempts        int
	Error           string
	SizeBytes       int64
	QueuedAt        string
	CopiedAt        string
}

const backupCopySelect = `
	SELECT c.backup_history_id,
	       CASE WHEN h.resource_kind = 'volume' THEN h.service_name || '/' || h.volume_name ELSE h.database_name END,
	       h.resource_kind, h.target_id, h.object_key, h.size_bytes, h.finished_at,
	       c.target_id, c.status, c.attempts, c.error, c.size_bytes, c.queued_at, c.copied_at
	FROM backup_copies c JOIN backup_history h ON h.id = c.backup_history_id`

func scanBackupCopy(s rowScanner) (BackupCopy, error) {
	var c BackupCopy
	err := s.Scan(&c.BackupHistoryID, &c.ResourceName, &c.ResourceKind, &c.SourceTargetID, &c.ObjectKey, &c.BackupSize, &c.BackupFinished,
		&c.TargetID, &c.Status, &c.Attempts, &c.Error, &c.SizeBytes, &c.QueuedAt, &c.CopiedAt)
	return c, err
}

// GetBackupCopySettings returns the second-destination setting.
func (db *DB) GetBackupCopySettings(ctx context.Context) (BackupCopySettings, error) {
	var s BackupCopySettings
	err := db.reader().QueryRowContext(ctx, `SELECT enabled, target_id, enabled_at FROM backup_copy_settings WHERE id = 1`).Scan(&s.Enabled, &s.TargetID, &s.EnabledAt)
	if err != nil {
		return BackupCopySettings{}, fmt.Errorf("store: get backup copy settings: %w", err)
	}
	return s, nil
}

// SetBackupCopySettings turns copying on for targetID or off. enabled_at only moves when copying is (re)enabled
// for a different destination, so older backups are never swept in by a toggle.
func (db *DB) SetBackupCopySettings(ctx context.Context, enabled bool, targetID, now string) error {
	if enabled && targetID == "" {
		return ErrBackupCopyTargetInvalid
	}
	prev, err := db.GetBackupCopySettings(ctx)
	if err != nil {
		return err
	}
	enabledAt := prev.EnabledAt
	if enabled && (!prev.Enabled || prev.TargetID != targetID) {
		enabledAt = now
	}
	if !enabled {
		targetID = prev.TargetID
	}
	if _, err := db.ExecContext(ctx, `UPDATE backup_copy_settings SET enabled = ?, target_id = ?, enabled_at = ? WHERE id = 1`, enabled, targetID, enabledAt); err != nil {
		return fmt.Errorf("store: set backup copy settings: %w", err)
	}
	return nil
}

// EnqueueBackupCopies queues every succeeded backup finished since `since` that lives on another destination.
func (db *DB) EnqueueBackupCopies(ctx context.Context, targetID, since, now string) error {
	_, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO backup_copies (backup_history_id, target_id, status, queued_at)
		SELECT id, ?, 'pending', ? FROM backup_history
		WHERE status = ? AND object_key != '' AND target_id != ? AND finished_at >= ?`,
		targetID, now, BackupStatusSucceeded, targetID, since)
	if err != nil {
		return fmt.Errorf("store: enqueue backup copies: %w", err)
	}
	return nil
}

// ClaimBackupCopies marks up to limit pending or failed copies as copying and returns them, oldest backup first.
func (db *DB) ClaimBackupCopies(ctx context.Context, limit int) ([]BackupCopy, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: claim backup copies: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, backupCopySelect+` WHERE c.status IN ('pending', 'failed') ORDER BY h.finished_at LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: claim backup copies: %w", err)
	}
	var out []BackupCopy
	for rows.Next() {
		c, err := scanBackupCopy(rows)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: scan backup copy: %w", err)
		}
		out = append(out, c)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: claim backup copies: %w", err)
	}
	for i := range out {
		if _, err := tx.ExecContext(ctx, `UPDATE backup_copies SET status = 'copying', attempts = attempts + 1 WHERE backup_history_id = ?`, out[i].BackupHistoryID); err != nil {
			return nil, fmt.Errorf("store: claim backup copy %q: %w", out[i].BackupHistoryID, err)
		}
		out[i].Status = BackupCopyCopying
		out[i].Attempts++
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: claim backup copies: %w", err)
	}
	return out, nil
}

// FinishBackupCopy records the outcome of one copy attempt.
func (db *DB) FinishBackupCopy(ctx context.Context, id string, size int64, errMsg, now string) error {
	status, copiedAt := BackupCopySucceeded, now
	if errMsg != "" {
		status, copiedAt = BackupCopyFailed, ""
	}
	if _, err := db.ExecContext(ctx, `UPDATE backup_copies SET status = ?, error = ?, size_bytes = ?, copied_at = ? WHERE backup_history_id = ?`, status, errMsg, size, copiedAt, id); err != nil {
		return fmt.Errorf("store: finish backup copy %q: %w", id, err)
	}
	return nil
}

// ResetStuckBackupCopies returns copies a crashed process left in copying to pending.
func (db *DB) ResetStuckBackupCopies(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, `UPDATE backup_copies SET status = 'pending' WHERE status = 'copying'`); err != nil {
		return fmt.Errorf("store: reset stuck backup copies: %w", err)
	}
	return nil
}

// ListBackupCopies returns the newest copies first.
func (db *DB) ListBackupCopies(ctx context.Context, limit int) ([]BackupCopy, error) {
	rows, err := db.reader().QueryContext(ctx, backupCopySelect+` ORDER BY h.finished_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list backup copies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BackupCopy
	for rows.Next() {
		c, err := scanBackupCopy(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan backup copy: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListUncopiedBackupCopies returns copies that have not succeeded, for lag alerting.
func (db *DB) ListUncopiedBackupCopies(ctx context.Context) ([]BackupCopy, error) {
	rows, err := db.reader().QueryContext(ctx, backupCopySelect+` WHERE c.status != 'succeeded' ORDER BY h.finished_at`)
	if err != nil {
		return nil, fmt.Errorf("store: list uncopied backups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []BackupCopy
	for rows.Next() {
		c, err := scanBackupCopy(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan backup copy: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
