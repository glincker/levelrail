package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrDatabaseInitScriptNotFound is returned by GetDatabaseInitScript,
// UpdateDatabaseInitScript, and DeleteDatabaseInitScript when id doesn't
// match any row.
var ErrDatabaseInitScriptNotFound = errors.New("store: database init script not found")

// DatabaseInitScript is one named SQL/shell file attached to a managed
// database, mounted into its container's /docker-entrypoint-initdb.d.
// See migrations/0080_database_init_scripts.sql for the full
// field-by-field reasoning.
type DatabaseInitScript struct {
	ID           string
	DatabaseName string
	Filename     string
	Content      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SaveDatabaseInitScript inserts a new init script row. ID is minted by
// the caller (internal/api), the same "generate before the INSERT"
// pattern SaveScheduledTask's own doc comment establishes.
func (db *DB) SaveDatabaseInitScript(ctx context.Context, s DatabaseInitScript) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_init_scripts (id, database_name, filename, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.ID, s.DatabaseName, s.Filename, s.Content, formatTime(s.CreatedAt), formatTime(s.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save database init script %q: %w", s.ID, err)
	}
	return nil
}

// GetDatabaseInitScript returns the init script with this ID, or
// ErrDatabaseInitScriptNotFound.
func (db *DB) GetDatabaseInitScript(ctx context.Context, id string) (DatabaseInitScript, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, database_name, filename, content, created_at, updated_at
		FROM database_init_scripts
		WHERE id = ?
	`, id)
	s, err := scanDatabaseInitScript(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return DatabaseInitScript{}, ErrDatabaseInitScriptNotFound
	}
	if err != nil {
		return DatabaseInitScript{}, fmt.Errorf("store: get database init script %q: %w", id, err)
	}
	return *s, nil
}

// ListDatabaseInitScripts returns every init script attached to
// databaseName, ordered by filename: the same order Docker's own
// entrypoint scripts execute them in (see this table's own migration
// comment for why filename, not a row-order column, is authoritative).
func (db *DB) ListDatabaseInitScripts(ctx context.Context, databaseName string) ([]DatabaseInitScript, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, database_name, filename, content, created_at, updated_at
		FROM database_init_scripts
		WHERE database_name = ?
		ORDER BY filename
	`, databaseName)
	if err != nil {
		return nil, fmt.Errorf("store: list database init scripts for %q: %w", databaseName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DatabaseInitScript
	for rows.Next() {
		s, err := scanDatabaseInitScript(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan database init script row: %w", err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate database init script rows: %w", err)
	}
	return out, nil
}

// UpdateDatabaseInitScript updates a script's own editable fields
// (Filename, Content): it never touches DatabaseName (an existing
// script cannot be reassigned to a different database; delete and
// recreate instead), the same reasoning UpdateScheduledTask's own doc
// comment gives for ServiceName. Returns ErrDatabaseInitScriptNotFound
// if id doesn't exist.
func (db *DB) UpdateDatabaseInitScript(ctx context.Context, id, filename, content string, updatedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE database_init_scripts
		SET filename = ?, content = ?, updated_at = ?
		WHERE id = ?
	`, filename, content, formatTime(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: update database init script %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrDatabaseInitScriptNotFound, "update database init script %q", id)
}

// DeleteDatabaseInitScript removes an init script row. Returns
// ErrDatabaseInitScriptNotFound if id doesn't exist.
func (db *DB) DeleteDatabaseInitScript(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM database_init_scripts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete database init script %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrDatabaseInitScriptNotFound, "delete database init script %q", id)
}

func scanDatabaseInitScript(scan func(dest ...any) error) (*DatabaseInitScript, error) {
	var (
		s                    DatabaseInitScript
		createdAt, updatedAt string
	)
	if err := scan(&s.ID, &s.DatabaseName, &s.Filename, &s.Content, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	var err error
	s.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	s.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &s, nil
}
