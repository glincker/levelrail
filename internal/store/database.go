package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Supported managed database engines, matching internal/spec's list:
// Postgres and Redis ship as first-class resources in the initial
// release.
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
	// NodeID: see DesiredService.NodeID's own doc comment, identical
	// meaning and identical "SaveDesiredDatabase never writes it, only
	// UpdateDatabaseNode does" exception below.
	NodeID string
}

// SaveDesiredDatabase creates or fully replaces the desired state for a
// named database, the same whole-record-replacement semantics as
// SaveDesiredService, including the identical NodeID exception
// SaveDesiredService's own doc comment explains.
func (db *DB) SaveDesiredDatabase(ctx context.Context, d DesiredDatabase) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO desired_databases (name, engine, version, node_id, updated_at)
		VALUES (?, ?, ?, '', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
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

// UpdateDatabaseNode is DesiredDatabase's counterpart to
// UpdateServiceNode.
func (db *DB) UpdateDatabaseNode(ctx context.Context, name, nodeID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_databases SET node_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nodeID, name)
	if err != nil {
		return fmt.Errorf("store: update node for database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update node for database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
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
		SELECT name, engine, version, node_id
		FROM desired_databases
		WHERE name = ?
	`, name).Scan(&d.Name, &d.Engine, &d.Version, &d.NodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDatabaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get desired database %q: %w", name, err)
	}
	return &d, nil
}

// DeleteDesiredDatabase removes a database's desired state, the same
// "not found" sentinel and "does not stop the running container itself"
// gap DeleteDesiredService's own doc comment documents, applied to a
// database instead of a service.
func (db *DB) DeleteDesiredDatabase(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM desired_databases WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete desired database %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete desired database %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// ListDesiredDatabases returns every saved database, ordered by name.
func (db *DB) ListDesiredDatabases(ctx context.Context) ([]DesiredDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, engine, version, node_id
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
		if err := rows.Scan(&d.Name, &d.Engine, &d.Version, &d.NodeID); err != nil {
			return nil, fmt.Errorf("store: scan desired database row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired database rows: %w", err)
	}
	return out, nil
}

// ListDesiredDatabasesByNode returns every saved database currently
// placed on nodeID, ordered by name. The database-kind counterpart to
// ListDesiredServicesByNode, same TASKS.md 3.7 drain/delete-guard
// callers.
func (db *DB) ListDesiredDatabasesByNode(ctx context.Context, nodeID string) ([]DesiredDatabase, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name, engine, version, node_id
		FROM desired_databases
		WHERE node_id = ?
		ORDER BY name
	`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("store: list desired databases for node %q: %w", nodeID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredDatabase
	for rows.Next() {
		var d DesiredDatabase
		if err := rows.Scan(&d.Name, &d.Engine, &d.Version, &d.NodeID); err != nil {
			return nil, fmt.Errorf("store: scan desired database row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired database rows: %w", err)
	}
	return out, nil
}
