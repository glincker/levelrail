package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Kinds a recently deleted row can hold.
const (
	RecentlyDeletedApp      = "app"
	RecentlyDeletedDatabase = "database"
)

// DeletedAppSecretsKey is the secrets namespace that holds a soft-deleted app's
// values for the undo window. App names cannot contain a slash, so it never collides.
func DeletedAppSecretsKey(name string) string { return "deleted-app/" + name }

// ErrRecentlyDeletedNotFound is returned when no undo entry exists for the name.
var ErrRecentlyDeletedNotFound = errors.New("store: recently deleted entry not found")

// RecentlyDeleted is a deleted app or database kept for the undo window.
// Snapshot is the JSON of its desired record; LastBackup is a JSON reference
// to its newest successful backup, empty when it had none.
type RecentlyDeleted struct {
	Kind       string
	Name       string
	Snapshot   string
	LastBackup string
	DeletedBy  string
	DeletedAt  time.Time
}

// AddRecentlyDeleted records a delete; a repeat for the same name replaces the older entry.
func (db *DB) AddRecentlyDeleted(ctx context.Context, e RecentlyDeleted) error {
	if e.DeletedAt.IsZero() {
		e.DeletedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO recently_deleted (kind, name, snapshot, last_backup, deleted_by, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (kind, name) DO UPDATE SET snapshot = excluded.snapshot, last_backup = excluded.last_backup,
			deleted_by = excluded.deleted_by, deleted_at = excluded.deleted_at
	`, e.Kind, e.Name, e.Snapshot, e.LastBackup, e.DeletedBy, e.DeletedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: add recently deleted %s %q: %w", e.Kind, e.Name, err)
	}
	return nil
}

// ListRecentlyDeleted returns every entry, newest delete first.
func (db *DB) ListRecentlyDeleted(ctx context.Context) ([]RecentlyDeleted, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT kind, name, snapshot, last_backup, deleted_by, deleted_at FROM recently_deleted ORDER BY deleted_at DESC, name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list recently deleted: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RecentlyDeleted
	for rows.Next() {
		e, err := scanRecentlyDeleted(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetRecentlyDeleted returns one entry or ErrRecentlyDeletedNotFound.
func (db *DB) GetRecentlyDeleted(ctx context.Context, kind, name string) (RecentlyDeleted, error) {
	row := db.reader().QueryRowContext(ctx, `
		SELECT kind, name, snapshot, last_backup, deleted_by, deleted_at FROM recently_deleted WHERE kind = ? AND name = ?
	`, kind, name)
	e, err := scanRecentlyDeleted(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return RecentlyDeleted{}, ErrRecentlyDeletedNotFound
	}
	return e, err
}

// DeleteRecentlyDeleted drops one entry; a missing one is not an error.
func (db *DB) DeleteRecentlyDeleted(ctx context.Context, kind, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM recently_deleted WHERE kind = ? AND name = ?`, kind, name); err != nil {
		return fmt.Errorf("store: delete recently deleted %s %q: %w", kind, name, err)
	}
	return nil
}

// PurgeRecentlyDeleted removes entries deleted before cutoff and returns how many went.
func (db *DB) PurgeRecentlyDeleted(ctx context.Context, cutoff time.Time) (int, error) {
	at := cutoff.UTC().Format(time.RFC3339Nano)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("store: purge recently deleted: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// The final purge is when kept env and config history goes.
	rows, err := tx.QueryContext(ctx, `SELECT name FROM recently_deleted WHERE kind = ? AND deleted_at < ?`, RecentlyDeletedApp, at)
	if err != nil {
		return 0, fmt.Errorf("store: purge recently deleted: %w", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("store: purge recently deleted: %w", err)
		}
		names = append(names, n)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("store: purge recently deleted: %w", err)
	}
	for _, n := range names {
		// The retained secret values go with the entry, live name or not.
		if err := deleteServiceSecretsTx(ctx, tx, DeletedAppSecretsKey(n)); err != nil {
			return 0, fmt.Errorf("store: purge recently deleted: %w", err)
		}
		// A restored app that kept its undo row must not lose its history.
		var live int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM desired_services WHERE name = ?`, n).Scan(&live); err != nil {
			return 0, fmt.Errorf("store: purge recently deleted: %w", err)
		}
		if live > 0 {
			continue
		}
		if err := dropAppRevisions(ctx, tx, n); err != nil {
			return 0, fmt.Errorf("store: purge recently deleted: %w", err)
		}
		// Function webhook and key-value data are kept for the undo window, so they go with the history.
		if err := dropFunctionData(ctx, tx, n); err != nil {
			return 0, fmt.Errorf("store: purge recently deleted: %w", err)
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM recently_deleted WHERE deleted_at < ?`, at)
	if err != nil {
		return 0, fmt.Errorf("store: purge recently deleted: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("store: purge recently deleted: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

type keepRevisionsKey struct{}

// WithKeptRevisions makes DeleteDesiredService leave the app's env and config history, for a soft delete.
func WithKeptRevisions(ctx context.Context) context.Context {
	return context.WithValue(ctx, keepRevisionsKey{}, true)
}

func revisionsKept(ctx context.Context) bool {
	v, _ := ctx.Value(keepRevisionsKey{}).(bool)
	return v
}

// PurgeAppRevisions drops the kept history of a deleted app, for a soft delete that could not reserve its name.
func (db *DB) PurgeAppRevisions(ctx context.Context, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: purge app revisions: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := dropAppRevisions(ctx, tx, name); err != nil {
		return fmt.Errorf("store: purge app revisions: %w", err)
	}
	return tx.Commit()
}

// dropAppRevisions removes env and config history; env snapshots are sealed per app name.
func dropAppRevisions(ctx context.Context, tx *sql.Tx, name string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_config_revisions WHERE app_name = ?`, name); err != nil {
		return fmt.Errorf("config revisions: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM env_revisions WHERE app_name = ?`, name); err != nil {
		return fmt.Errorf("env revisions: %w", err)
	}
	return nil
}

// dropFunctionData removes a function's webhook row and key-value data and token.
func dropFunctionData(ctx context.Context, tx *sql.Tx, name string) error {
	for _, q := range []string{
		`DELETE FROM function_webhooks WHERE function = ?`,
		`DELETE FROM function_kv WHERE namespace = ?`,
		`DELETE FROM function_kv_tokens WHERE namespace = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, name); err != nil {
			return fmt.Errorf("function data: %w", err)
		}
	}
	return nil
}

func scanRecentlyDeleted(scan func(dest ...any) error) (RecentlyDeleted, error) {
	var e RecentlyDeleted
	var at string
	if err := scan(&e.Kind, &e.Name, &e.Snapshot, &e.LastBackup, &e.DeletedBy, &at); err != nil {
		return RecentlyDeleted{}, fmt.Errorf("store: scan recently deleted: %w", err)
	}
	e.DeletedAt, _ = time.Parse(time.RFC3339Nano, at)
	return e, nil
}
