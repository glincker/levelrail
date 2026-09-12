package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

const previewEphemeralDatabaseIDPrefix = "pedb_"

// NewPreviewEphemeralDatabaseID mints a random ephemeral database
// tracking row ID, the same scheme NewPreviewEnvironmentID already
// establishes for its own webhook-minted identifier.
func NewPreviewEphemeralDatabaseID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate preview ephemeral database id: %w", err)
	}
	return previewEphemeralDatabaseIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Preview ephemeral database statuses (migrations/0092_preview_ephemeral_databases.sql).
// Provisioned is the only state this row is ever saved in; TeardownFailed
// is set in place, never SaveDesiredDatabase, so a retry can find exactly
// which ones are still owed a teardown.
const (
	PreviewEphemeralDatabaseStatusProvisioned    = "provisioned"
	PreviewEphemeralDatabaseStatusTeardownFailed = "teardown_failed"
)

// ErrPreviewEphemeralDatabaseNotFound is returned by
// GetPreviewEphemeralDatabaseByPreviewAndKey when no row matches.
var ErrPreviewEphemeralDatabaseNotFound = errors.New("store: preview ephemeral database not found")

// PreviewEphemeralDatabase links a PreviewEnvironment to one
// database.Controller-managed DesiredDatabase provisioned just for it:
// one row per (PreviewEnvironmentID, SourceKey), SourceKey being the
// app.yaml databases: key (spec.Database.EphemeralInPreviews) this
// instance stands in for. DatabaseName is a desired_databases.name this
// row's own container reconciles against through the exact same
// dynamicSource/database.Controller path every other managed database
// uses; this table adds no reconciliation logic of its own, only the
// linkage a preview's teardown needs to find and remove it.
type PreviewEphemeralDatabase struct {
	ID                   string
	PreviewEnvironmentID string
	DatabaseName         string
	SourceKey            string
	Engine               string
	Version              string
	Status               string
	StatusReason         string
	CreatedAt            string
	UpdatedAt            string
}

// SavePreviewEphemeralDatabase inserts a new tracking row. Insert-only:
// callers look the row up first (GetPreviewEphemeralDatabaseByPreviewAndKey)
// and only call this on a genuine miss, the same idempotent-creation
// shape deployPreviewEnvironment already uses for preview_environments
// itself, so a PR synchronize event that re-runs provisioning never
// mints a second row for the same (preview, source key) pair.
func (db *DB) SavePreviewEphemeralDatabase(ctx context.Context, p PreviewEphemeralDatabase) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO preview_ephemeral_databases
			(id, preview_environment_id, database_name, source_key, engine, version, status, status_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.PreviewEnvironmentID, p.DatabaseName, p.SourceKey, p.Engine, p.Version, p.Status,
		sql.NullString{String: p.StatusReason, Valid: p.StatusReason != ""},
		p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save preview ephemeral database %q: %w", p.ID, err)
	}
	return nil
}

// GetPreviewEphemeralDatabaseByPreviewAndKey returns the ephemeral
// database tracking row for (previewEnvironmentID, sourceKey), or
// ErrPreviewEphemeralDatabaseNotFound: provisioning's own idempotency
// check, matching the table's own unique index.
func (db *DB) GetPreviewEphemeralDatabaseByPreviewAndKey(ctx context.Context, previewEnvironmentID, sourceKey string) (*PreviewEphemeralDatabase, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+previewEphemeralDatabaseColumns+`
		FROM preview_ephemeral_databases WHERE preview_environment_id = ? AND source_key = ?
	`, previewEnvironmentID, sourceKey)
	p, err := scanPreviewEphemeralDatabase(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPreviewEphemeralDatabaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get preview ephemeral database for %q key %q: %w", previewEnvironmentID, sourceKey, err)
	}
	return p, nil
}

// ListPreviewEphemeralDatabasesByPreview returns every ephemeral database
// tracking row for previewEnvironmentID, for the API's own status
// surface and for teardown to find everything it owns.
func (db *DB) ListPreviewEphemeralDatabasesByPreview(ctx context.Context, previewEnvironmentID string) ([]PreviewEphemeralDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+previewEphemeralDatabaseColumns+`
		FROM preview_ephemeral_databases WHERE preview_environment_id = ? ORDER BY source_key ASC
	`, previewEnvironmentID)
	if err != nil {
		return nil, fmt.Errorf("store: list preview ephemeral databases for %q: %w", previewEnvironmentID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PreviewEphemeralDatabase
	for rows.Next() {
		p, err := scanPreviewEphemeralDatabase(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan preview ephemeral database row: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate preview ephemeral database rows: %w", err)
	}
	return out, nil
}

// UpdatePreviewEphemeralDatabaseStatus sets id's status/status_reason in
// place, the same "record why, keep the row for a retry" shape
// finishPreviewFailed already establishes for preview_environments
// itself. Returns ErrPreviewEphemeralDatabaseNotFound if id doesn't
// exist.
func (db *DB) UpdatePreviewEphemeralDatabaseStatus(ctx context.Context, id, status, statusReason, updatedAt string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE preview_ephemeral_databases SET status = ?, status_reason = ?, updated_at = ?
		WHERE id = ?
	`, status, sql.NullString{String: statusReason, Valid: statusReason != ""}, updatedAt, id)
	if err != nil {
		return fmt.Errorf("store: update preview ephemeral database status %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update preview ephemeral database status %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrPreviewEphemeralDatabaseNotFound
	}
	return nil
}

// DeletePreviewEphemeralDatabase removes a tracking row once its
// database has actually been torn down; returns
// ErrPreviewEphemeralDatabaseNotFound if id doesn't exist.
func (db *DB) DeletePreviewEphemeralDatabase(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM preview_ephemeral_databases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete preview ephemeral database %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete preview ephemeral database %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrPreviewEphemeralDatabaseNotFound
	}
	return nil
}

const previewEphemeralDatabaseColumns = "id, preview_environment_id, database_name, source_key, engine, version, status, status_reason, created_at, updated_at"

func scanPreviewEphemeralDatabase(scan func(dest ...any) error) (*PreviewEphemeralDatabase, error) {
	var (
		p            PreviewEphemeralDatabase
		statusReason sql.NullString
	)
	if err := scan(&p.ID, &p.PreviewEnvironmentID, &p.DatabaseName, &p.SourceKey, &p.Engine, &p.Version, &p.Status, &statusReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.StatusReason = statusReason.String
	return &p, nil
}
