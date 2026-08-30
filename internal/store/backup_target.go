package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Backup target providers migrations/0018_backup_targets.sql's CHECK
// constraint accepts. "aws" leaves Endpoint empty (the AWS SDK's default
// per-region resolver already knows the right host); "r2" and "custom"
// both require an explicit Endpoint, the only structural difference
// between them being that "r2" exists as its own value so the API and
// frontend can show a Cloudflare-specific label instead of a bare URL.
const (
	BackupProviderAWS    = "aws"
	BackupProviderR2     = "r2"
	BackupProviderCustom = "custom"
)

// ErrBackupTargetNotFound is returned by GetBackupTarget and
// DeleteBackupTarget when id doesn't match any row.
var ErrBackupTargetNotFound = errors.New("store: backup target not found")

// BackupTarget is a connected S3-compatible bucket a database backup can
// be uploaded to. No credential fields: see this table's own migration
// comment. A caller resolving credentials does so separately through
// internal/secrets, keyed by BackupTargetSecretsKey(target.ID).
type BackupTarget struct {
	ID        string
	Name      string
	Provider  string
	Endpoint  string
	Region    string
	Bucket    string
	CreatedAt string
}

// BackupTargetSecretsKey is the internal/secrets serviceName a backup
// target's access key ID and secret access key are stored under
// (envKeys "access_key_id" and "secret_access_key"). A function, not a
// constant format string inlined at each call site, so internal/api and
// internal/backup can never drift into computing this two different
// ways.
func BackupTargetSecretsKey(targetID string) string {
	return "backup-target/" + targetID
}

// SaveBackupTarget inserts a new backup target row. IDs are minted by
// the caller (internal/api, the same "generate before the INSERT"
// pattern api_token/node join tokens already use) so the caller can
// store credentials under the same ID before or after this call without
// a round trip to discover what ID got assigned.
func (db *DB) SaveBackupTarget(ctx context.Context, t BackupTarget) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO backup_targets (id, name, provider, endpoint, region, bucket, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.Provider, t.Endpoint, t.Region, t.Bucket, t.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save backup target %q: %w", t.ID, err)
	}
	return nil
}

// GetBackupTarget returns the backup target with this ID, or
// ErrBackupTargetNotFound.
func (db *DB) GetBackupTarget(ctx context.Context, id string) (BackupTarget, error) {
	var t BackupTarget
	err := db.QueryRowContext(ctx, `
		SELECT id, name, provider, endpoint, region, bucket, created_at
		FROM backup_targets
		WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &t.Provider, &t.Endpoint, &t.Region, &t.Bucket, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BackupTarget{}, ErrBackupTargetNotFound
	}
	if err != nil {
		return BackupTarget{}, fmt.Errorf("store: get backup target %q: %w", id, err)
	}
	return t, nil
}

// ListBackupTargets returns every backup target, oldest first (creation
// order, the same order a settings page listing them would want).
func (db *DB) ListBackupTargets(ctx context.Context) ([]BackupTarget, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, provider, endpoint, region, bucket, created_at
		FROM backup_targets
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list backup targets: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []BackupTarget
	for rows.Next() {
		var t BackupTarget
		if err := rows.Scan(&t.ID, &t.Name, &t.Provider, &t.Endpoint, &t.Region, &t.Bucket, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan backup target row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate backup target rows: %w", err)
	}
	return out, nil
}

// UpdateBackupTarget replaces name/provider/endpoint/region/bucket for an
// existing row, the same full-replace contract UpdateScheduledTask uses.
// Credentials are not this function's concern: a caller rotating them
// writes to internal/secrets separately, under the same
// BackupTargetSecretsKey(id) this row's ID has always used.
// Returns ErrBackupTargetNotFound if id doesn't exist.
func (db *DB) UpdateBackupTarget(ctx context.Context, id, name, provider, endpoint, region, bucket string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE backup_targets
		SET name = ?, provider = ?, endpoint = ?, region = ?, bucket = ?
		WHERE id = ?
	`, name, provider, endpoint, region, bucket, id)
	if err != nil {
		return fmt.Errorf("store: update backup target %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrBackupTargetNotFound, "update backup target %q", id)
}

// DeleteBackupTarget removes a backup target row. It does not touch
// internal/secrets: the caller (internal/api's handleDeleteBackupTarget)
// deletes the credential secret separately, after this succeeds, the
// same "store row is the success signal, secret cleanup follows it"
// ordering used everywhere else credentials and store rows are paired.
// Returns ErrBackupTargetNotFound if id doesn't exist. Foreign key
// enforcement is on (store.go's "_pragma foreign_keys(ON)"), so deleting
// a target any backup_history row still references fails with a
// constraint error rather than silently orphaning history rows or
// cascading a delete a user didn't ask for; the caller surfaces that as
// a real error, not a bug to work around here.
func (db *DB) DeleteBackupTarget(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM backup_targets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete backup target %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete backup target %q: %w", id, err)
	}
	if n == 0 {
		return ErrBackupTargetNotFound
	}
	return nil
}
