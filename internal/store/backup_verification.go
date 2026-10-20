package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Backup verification statuses migrations/0069_backup_verification.sql's
// CHECK constraint accepts, mirroring BackupStatusRunning/Succeeded/Failed's
// own "running" written first, before real work happens" reasoning
// (backup_history.go): a verification attempt that downloads a large
// object can take a while, so a row exists from the moment it starts, not
// only once it finishes. "passed"/"failed" describe whether the backup
// itself checked out, not whether the verification process ran without
// error; VerifyBackup (internal/backup) folds both into the same failed
// status, since either one means an operator cannot trust this backup
// without more investigation.
const (
	BackupVerificationStatusRunning = "running"
	BackupVerificationStatusPassed  = "passed"
	BackupVerificationStatusFailed  = "failed"
)

// ErrBackupVerificationNotFound is returned by FinishBackupVerification
// when id doesn't match any row, and by GetLatestBackupVerification when
// backupHistoryID has never been verified.
var ErrBackupVerificationNotFound = errors.New("store: backup verification not found")

// BackupVerification is one attempt to confirm a previously succeeded
// backup_history row's stored object is still intact: re-downloaded,
// re-hashed, and checked against what was recorded at backup time,
// without ever attempting a live restore. Error is empty unless Status is
// BackupVerificationStatusFailed.
type BackupVerification struct {
	ID              string
	BackupHistoryID string
	Status          string
	ChecksumMatch   bool
	SizeMatch       bool
	FormatValid     bool
	DownloadedBytes int64
	Error           string
	CheckedBy       string
	StartedAt       string
	FinishedAt      string
}

// StartBackupVerification records a verification attempt beginning,
// status BackupVerificationStatusRunning, the same "write running first"
// reasoning this file's own status-constants comment gives.
func (db *DB) StartBackupVerification(ctx context.Context, v BackupVerification) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_verification (id, backup_history_id, status, checked_by, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, '')
	`, v.ID, v.BackupHistoryID, BackupVerificationStatusRunning, v.CheckedBy, v.StartedAt)
	if err != nil {
		return fmt.Errorf("store: start backup verification %q: %w", v.ID, err)
	}
	return nil
}

// FinishBackupVerification updates a running verification row to its
// final status, recording each individual check's own outcome
// (checksumMatch, sizeMatch, formatValid) alongside the overall status, so
// a failure can be explained (which check failed), not just reported.
func (db *DB) FinishBackupVerification(ctx context.Context, id, status string, checksumMatch, sizeMatch, formatValid bool, downloadedBytes int64, errMsg, finishedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE backup_verification
		SET status = ?, checksum_match = ?, size_match = ?, format_valid = ?, downloaded_bytes = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, status, boolToInt(checksumMatch), boolToInt(sizeMatch), boolToInt(formatValid), downloadedBytes, errMsg, finishedAt, id)
	if err != nil {
		return fmt.Errorf("store: finish backup verification %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrBackupVerificationNotFound, "finish backup verification %q", id)
}

// ListBackupVerifications returns up to limit verification attempts for
// backupHistoryID, newest first.
func (db *DB) ListBackupVerifications(ctx context.Context, backupHistoryID string, limit int) ([]BackupVerification, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, backup_history_id, status, checksum_match, size_match, format_valid, downloaded_bytes, error, checked_by, started_at, finished_at
		FROM backup_verification
		WHERE backup_history_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, backupHistoryID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list backup verifications for %q: %w", backupHistoryID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []BackupVerification
	for rows.Next() {
		v, err := scanBackupVerification(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan backup verification row: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate backup verification rows: %w", err)
	}
	return out, nil
}

// GetLatestBackupVerification returns the most recently started
// verification attempt for backupHistoryID, or ErrBackupVerificationNotFound
// if it has never been verified. Used by the backup history detail view
// (and the CLI's own "backups verifications" summary) to show a single
// current status without listing full history.
func (db *DB) GetLatestBackupVerification(ctx context.Context, backupHistoryID string) (BackupVerification, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, backup_history_id, status, checksum_match, size_match, format_valid, downloaded_bytes, error, checked_by, started_at, finished_at
		FROM backup_verification
		WHERE backup_history_id = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, backupHistoryID)
	v, err := scanBackupVerification(row)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupVerification{}, ErrBackupVerificationNotFound
	}
	if err != nil {
		return BackupVerification{}, fmt.Errorf("store: get latest backup verification for %q: %w", backupHistoryID, err)
	}
	return v, nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows, letting
// scanBackupVerification back both GetLatestBackupVerification's
// single-row query and ListBackupVerifications' multi-row one with one
// scan implementation instead of two copies that would drift.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBackupVerification(row rowScanner) (BackupVerification, error) {
	var v BackupVerification
	var checksumMatch, sizeMatch, formatValid int
	err := row.Scan(&v.ID, &v.BackupHistoryID, &v.Status, &checksumMatch, &sizeMatch, &formatValid, &v.DownloadedBytes, &v.Error, &v.CheckedBy, &v.StartedAt, &v.FinishedAt)
	if err != nil {
		return BackupVerification{}, err
	}
	v.ChecksumMatch = intToBool(checksumMatch)
	v.SizeMatch = intToBool(sizeMatch)
	v.FormatValid = intToBool(formatValid)
	return v, nil
}
