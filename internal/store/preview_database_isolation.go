package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

const previewDatabaseIsolationIDPrefix = "pdbi_"

// NewPreviewDatabaseIsolationID mints a random tracking row ID, the same
// scheme NewPreviewEphemeralDatabaseID already establishes.
func NewPreviewDatabaseIsolationID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate preview database isolation id: %w", err)
	}
	return previewDatabaseIsolationIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Preview database isolation statuses (migrations/0232_preview_database_isolations.sql).
const (
	PreviewDatabaseIsolationStatusProvisioned    = "provisioned"
	PreviewDatabaseIsolationStatusTeardownFailed = "teardown_failed"
)

// ErrPreviewDatabaseIsolationNotFound is returned when no row matches.
var ErrPreviewDatabaseIsolationNotFound = errors.New("store: preview database isolation not found")

// PreviewDatabaseIsolation links a PreviewEnvironment to an isolated
// Postgres role provisioned on an existing (not preview-owned)
// DesiredDatabase: one row per (PreviewEnvironmentID, SourceKey).
// DatabaseName is the existing desired_databases.name the role was
// created on; this table owns nothing about that database's own
// lifecycle, only the role's.
type PreviewDatabaseIsolation struct {
	ID                   string
	PreviewEnvironmentID string
	DatabaseName         string
	SourceKey            string
	RoleName             string
	SecretEnvKey         string
	Status               string
	StatusReason         string
	CreatedAt            string
	UpdatedAt            string
}

// SavePreviewDatabaseIsolation inserts a new tracking row.
func (db *DB) SavePreviewDatabaseIsolation(ctx context.Context, p PreviewDatabaseIsolation) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO preview_database_isolations
			(id, preview_environment_id, database_name, source_key, role_name, secret_env_key, status, status_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.PreviewEnvironmentID, p.DatabaseName, p.SourceKey, p.RoleName, p.SecretEnvKey, p.Status,
		sql.NullString{String: p.StatusReason, Valid: p.StatusReason != ""},
		p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save preview database isolation %q: %w", p.ID, err)
	}
	return nil
}

// GetPreviewDatabaseIsolationByPreviewAndKey returns the isolation
// tracking row for (previewEnvironmentID, sourceKey), or
// ErrPreviewDatabaseIsolationNotFound: provisioning's own idempotency
// check.
func (db *DB) GetPreviewDatabaseIsolationByPreviewAndKey(ctx context.Context, previewEnvironmentID, sourceKey string) (*PreviewDatabaseIsolation, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+previewDatabaseIsolationColumns+`
		FROM preview_database_isolations WHERE preview_environment_id = ? AND source_key = ?
	`, previewEnvironmentID, sourceKey)
	p, err := scanPreviewDatabaseIsolation(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPreviewDatabaseIsolationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get preview database isolation for %q key %q: %w", previewEnvironmentID, sourceKey, err)
	}
	return p, nil
}

// ListPreviewDatabaseIsolationsByPreview returns every isolation
// tracking row for previewEnvironmentID.
func (db *DB) ListPreviewDatabaseIsolationsByPreview(ctx context.Context, previewEnvironmentID string) ([]PreviewDatabaseIsolation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+previewDatabaseIsolationColumns+`
		FROM preview_database_isolations WHERE preview_environment_id = ? ORDER BY source_key ASC
	`, previewEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("store: list preview database isolations for %q: %w", previewEnvironmentID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PreviewDatabaseIsolation
	for rows.Next() {
		p, err := scanPreviewDatabaseIsolation(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan preview database isolation row: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate preview database isolation rows: %w", err)
	}
	return out, nil
}

// UpdatePreviewDatabaseIsolationStatus sets id's status/status_reason in
// place. Returns ErrPreviewDatabaseIsolationNotFound if id doesn't exist.
func (db *DB) UpdatePreviewDatabaseIsolationStatus(ctx context.Context, id, status, statusReason, updatedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE preview_database_isolations SET status = ?, status_reason = ?, updated_at = ?
		WHERE id = ?
	`, status, sql.NullString{String: statusReason, Valid: statusReason != ""}, updatedAt, id)
	if err != nil {
		return fmt.Errorf("store: update preview database isolation status %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update preview database isolation status %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrPreviewDatabaseIsolationNotFound
	}
	return nil
}

// DeletePreviewDatabaseIsolation removes a tracking row once its role has
// actually been dropped; returns ErrPreviewDatabaseIsolationNotFound if
// id doesn't exist.
func (db *DB) DeletePreviewDatabaseIsolation(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM preview_database_isolations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete preview database isolation %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete preview database isolation %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrPreviewDatabaseIsolationNotFound
	}
	return nil
}

const previewDatabaseIsolationColumns = "id, preview_environment_id, database_name, source_key, role_name, secret_env_key, status, status_reason, created_at, updated_at"

func scanPreviewDatabaseIsolation(scan func(dest ...any) error) (*PreviewDatabaseIsolation, error) {
	var (
		p            PreviewDatabaseIsolation
		statusReason sql.NullString
	)
	if err := scan(&p.ID, &p.PreviewEnvironmentID, &p.DatabaseName, &p.SourceKey, &p.RoleName, &p.SecretEnvKey, &p.Status, &statusReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.StatusReason = statusReason.String
	return &p, nil
}
