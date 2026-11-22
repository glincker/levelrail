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
	res, err := db.ExecContext(ctx, `DELETE FROM recently_deleted WHERE deleted_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: purge recently deleted: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
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
