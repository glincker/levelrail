package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Supported managed database engines, matching internal/spec's list
// (CLAUDE.md 6 Phase 1: "Managed Postgres and Redis as first-class
// resources").
const (
	EnginePostgres = "postgres"
	EngineRedis    = "redis"
)

// DesiredDatabase is what a future database controller (TASKS.md 1.8)
// reconciles a managed database container against. No credentials field:
// see the comment in migrations/0003_desired_databases.sql for why.
type DesiredDatabase struct {
	Name    string
	Engine  string
	Version string
}

// SaveDesiredDatabase creates or fully replaces the desired state for a
// named database, the same whole-record-replacement semantics as
// SaveDesiredService.
func (db *DB) SaveDesiredDatabase(ctx context.Context, d DesiredDatabase) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO desired_databases (name, engine, version, updated_at)
		VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (name) DO UPDATE SET
			engine = excluded.engine,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, d.Name, d.Engine, d.Version)
	if err != nil {
		return fmt.Errorf("store: save desired database %q: %w", d.Name, err)
	}
	return nil
}

// ErrDatabaseNotFound is returned by GetDesiredDatabase when no database
// has that name.
var ErrDatabaseNotFound = errors.New("store: database not found")

// GetDesiredDatabase returns the desired state for name, or
// ErrDatabaseNotFound if no such database has been saved.
func (db *DB) GetDesiredDatabase(ctx context.Context, name string) (*DesiredDatabase, error) {
	var d DesiredDatabase
	err := db.QueryRowContext(ctx, `
		SELECT name, engine, version
		FROM desired_databases
		WHERE name = ?
	`, name).Scan(&d.Name, &d.Engine, &d.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDatabaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get desired database %q: %w", name, err)
	}
	return &d, nil
}

// ListDesiredDatabases returns every saved database, ordered by name.
func (db *DB) ListDesiredDatabases(ctx context.Context) ([]DesiredDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, engine, version
		FROM desired_databases
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list desired databases: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredDatabase
	for rows.Next() {
		var d DesiredDatabase
		if err := rows.Scan(&d.Name, &d.Engine, &d.Version); err != nil {
			return nil, fmt.Errorf("store: scan desired database row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired database rows: %w", err)
	}
	return out, nil
}
