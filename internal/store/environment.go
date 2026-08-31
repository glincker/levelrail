package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrEnvironmentNotFound is returned by GetEnvironment and
// DeleteEnvironment when id doesn't match any row.
var ErrEnvironmentNotFound = errors.New("store: environment not found")

// Environment labels a service (e.g. "staging", "production") within
// one project (migrations/0054). Unlike Project/Organization,
// environments are owned by their project: deleting the project cascades.
//
// Protected (migrations/0070) requires confirm: true on a deploy,
// rollback, or promotion targeting an app tagged with this environment
// (internal/api's deploys.go and promote.go).
type Environment struct {
	ID        string
	ProjectID string
	Name      string
	Protected bool
	CreatedAt string
}

// SaveEnvironment inserts a new environment row, insert-only.
func (db *DB) SaveEnvironment(ctx context.Context, e Environment) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO environments (id, project_id, name, protected, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, e.ID, e.ProjectID, e.Name, e.Protected, e.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save environment %q: %w", e.ID, err)
	}
	return nil
}

// GetEnvironment returns the environment with this ID, or
// ErrEnvironmentNotFound.
func (db *DB) GetEnvironment(ctx context.Context, id string) (Environment, error) {
	var e Environment
	err := db.QueryRowContext(ctx, `
		SELECT id, project_id, name, protected, created_at
		FROM environments
		WHERE id = ?
	`, id).Scan(&e.ID, &e.ProjectID, &e.Name, &e.Protected, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Environment{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return Environment{}, fmt.Errorf("store: get environment %q: %w", id, err)
	}
	return e, nil
}

// ListEnvironmentsByProject returns every environment for one project,
// oldest first.
func (db *DB) ListEnvironmentsByProject(ctx context.Context, projectID string) ([]Environment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, project_id, name, protected, created_at
		FROM environments
		WHERE project_id = ?
		ORDER BY created_at
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list environments for project %q: %w", projectID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []Environment
	for rows.Next() {
		var e Environment
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Protected, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan environment row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate environment rows: %w", err)
	}
	return out, nil
}

// SetEnvironmentProtected updates one environment's protected flag.
// Returns ErrEnvironmentNotFound if id doesn't match any row.
func (db *DB) SetEnvironmentProtected(ctx context.Context, id string, protected bool) error {
	res, err := db.ExecContext(ctx, `UPDATE environments SET protected = ? WHERE id = ?`, protected, id)
	if err != nil {
		return fmt.Errorf("store: set environment %q protected: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set environment %q protected: %w", id, err)
	}
	if n == 0 {
		return ErrEnvironmentNotFound
	}
	return nil
}

// DeleteEnvironment removes an environment row. desired_services.
// environment_id is ON DELETE SET NULL (migrations/0054), so every
// service tagged with this environment is left running, untagged again.
func (db *DB) DeleteEnvironment(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM environments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete environment %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete environment %q: %w", id, err)
	}
	if n == 0 {
		return ErrEnvironmentNotFound
	}
	return nil
}

// SetServiceEnvironment assigns or clears (envID == "") a service's
// environment. Returns ErrServiceNotFound if the service doesn't exist.
func (db *DB) SetServiceEnvironment(ctx context.Context, serviceName, envID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET environment_id = ? WHERE name = ?
	`, sql.NullString{String: envID, Valid: envID != ""}, serviceName)
	if err != nil {
		return fmt.Errorf("store: set service %q environment: %w", serviceName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set service %q environment: %w", serviceName, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}
