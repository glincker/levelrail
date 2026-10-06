package store

import (
	"context"
	"database/sql"
	"fmt"
)

// EnvironmentWithCounts is an environment plus how many apps and databases
// are tagged with it.
type EnvironmentWithCounts struct {
	Environment
	AppCount      int
	DatabaseCount int
}

// ListAllEnvironments returns every environment, global and per project,
// with tagged app and database counts, ordered by sort_order then creation.
func (db *DB) ListAllEnvironments(ctx context.Context) ([]EnvironmentWithCounts, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.project_id, e.name, e.protected, e.created_at, e.kind, e.scope, e.sort_order,
			(SELECT COUNT(*) FROM desired_services s WHERE s.environment_id = e.id),
			(SELECT COUNT(*) FROM desired_databases d WHERE d.environment_id = e.id)
		FROM environments e
		ORDER BY e.sort_order, e.created_at, e.id
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list all environments: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var out []EnvironmentWithCounts
	for rows.Next() {
		var e EnvironmentWithCounts
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Protected, &e.CreatedAt, &e.Kind, &e.Scope, &e.SortOrder, &e.AppCount, &e.DatabaseCount); err != nil {
			return nil, fmt.Errorf("store: scan environment row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate environment rows: %w", err)
	}
	return out, nil
}

// EnvironmentPatch carries the editable fields of an environment, nil
// meaning unchanged.
type EnvironmentPatch struct {
	Name      *string
	Kind      *string
	SortOrder *int
	Protected *bool
}

// UpdateEnvironment applies patch to one environment, returning
// ErrEnvironmentNotFound when id matches no row.
func (db *DB) UpdateEnvironment(ctx context.Context, id string, patch EnvironmentPatch) error {
	res, err := db.ExecContext(ctx, `
		UPDATE environments SET
			name = COALESCE(?, name),
			kind = COALESCE(?, kind),
			sort_order = COALESCE(?, sort_order),
			protected = COALESCE(?, protected)
		WHERE id = ?
	`, nullString(patch.Name), nullString(patch.Kind), nullInt(patch.SortOrder), nullBool(patch.Protected), id)
	if err != nil {
		return fmt.Errorf("store: update environment %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update environment %q: %w", id, err)
	}
	if n == 0 {
		return ErrEnvironmentNotFound
	}
	return nil
}

func nullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func nullInt(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func nullBool(p *bool) sql.NullBool {
	if p == nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: *p, Valid: true}
}

// SetDatabaseEnvironment assigns or clears (envID == "") a database's
// environment, returning ErrDatabaseNotFound for an unknown name.
func (db *DB) SetDatabaseEnvironment(ctx context.Context, databaseName, envID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_databases SET environment_id = ? WHERE name = ?
	`, sql.NullString{String: envID, Valid: envID != ""}, databaseName)
	if err != nil {
		return fmt.Errorf("store: set database %q environment: %w", databaseName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set database %q environment: %w", databaseName, err)
	}
	if n == 0 {
		return ErrDatabaseNotFound
	}
	return nil
}

// DatabaseNamesInEnvironment returns the databases tagged with envID, as a set.
func (db *DB) DatabaseNamesInEnvironment(ctx context.Context, envID string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM desired_databases WHERE environment_id = ?`, envID)
	if err != nil {
		return nil, fmt.Errorf("store: list databases in environment %q: %w", envID, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan database name: %w", err)
		}
		out[name] = true
	}
	return out, rows.Err()
}

// EnvironmentMemberCounts returns how many apps and databases are tagged
// with envID.
func (db *DB) EnvironmentMemberCounts(ctx context.Context, envID string) (apps, databases int, err error) {
	err = db.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM desired_services WHERE environment_id = ?),
			(SELECT COUNT(*) FROM desired_databases WHERE environment_id = ?)
	`, envID, envID).Scan(&apps, &databases)
	if err != nil {
		return 0, 0, fmt.Errorf("store: count members of environment %q: %w", envID, err)
	}
	return apps, databases, nil
}

// MoveEnvironmentMembers retags every app and database in fromID to toID in
// one transaction.
func (db *DB) MoveEnvironmentMembers(ctx context.Context, fromID, toID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: move environment members: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET environment_id = ? WHERE environment_id = ?`, toID, fromID); err != nil {
		return fmt.Errorf("store: move services from environment %q: %w", fromID, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_databases SET environment_id = ? WHERE environment_id = ?`, toID, fromID); err != nil {
		return fmt.Errorf("store: move databases from environment %q: %w", fromID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: move environment members: %w", err)
	}
	return nil
}
