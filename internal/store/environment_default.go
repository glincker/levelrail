package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrDefaultEnvironment is returned when a delete targets a project's default environment.
var ErrDefaultEnvironment = errors.New("store: environment is the project default")

// DefaultEnvironmentID returns the project's default environment id, or "" when it has none.
func (db *DB) DefaultEnvironmentID(ctx context.Context, projectID string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `
		SELECT id FROM environments WHERE project_id = ? AND is_default = 1 AND scope = 'project'
	`, projectID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: default environment of project %q: %w", projectID, err)
	}
	return id, nil
}

// DefaultEnvironmentIDs maps project id to default environment id for every project that has one.
func (db *DB) DefaultEnvironmentIDs(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT project_id, id FROM environments WHERE is_default = 1 AND scope = 'project'`)
	if err != nil {
		return nil, fmt.Errorf("store: list default environments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var p, e string
		if err := rows.Scan(&p, &e); err != nil {
			return nil, fmt.Errorf("store: scan default environment: %w", err)
		}
		out[p] = e
	}
	return out, rows.Err()
}

// SaveDefaultEnvironment inserts e as its project's default, clearing any previous default in one transaction.
func (db *DB) SaveDefaultEnvironment(ctx context.Context, e Environment) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save default environment: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE environments SET is_default = 0 WHERE project_id = ? AND is_default = 1`, e.ProjectID); err != nil {
		return fmt.Errorf("store: save default environment: clear: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO environments (id, project_id, name, protected, created_at, kind, scope, sort_order, is_default)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)
	`, e.ID, e.ProjectID, e.Name, e.Protected, e.CreatedAt, kindOrCustom(e.Kind), EnvironmentScopeProject, e.SortOrder); err != nil {
		return fmt.Errorf("store: save default environment %q: %w", e.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save default environment: commit: %w", err)
	}
	return nil
}

// SetProjectDefaultEnvironment makes envID the project's default and clears the previous one atomically.
// An environment of another project, or a global one, answers ErrEnvironmentNotFound.
func (db *DB) SetProjectDefaultEnvironment(ctx context.Context, projectID, envID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set default environment: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var found string
	err = tx.QueryRowContext(ctx, `SELECT id FROM environments WHERE id = ? AND project_id = ? AND scope = 'project'`, envID, projectID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrEnvironmentNotFound
	}
	if err != nil {
		return fmt.Errorf("store: set default environment: look up: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE environments SET is_default = 0 WHERE project_id = ? AND is_default = 1 AND id != ?`, projectID, envID); err != nil {
		return fmt.Errorf("store: set default environment: clear: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE environments SET is_default = 1 WHERE id = ?`, envID); err != nil {
		return fmt.Errorf("store: set default environment: set: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set default environment: commit: %w", err)
	}
	return nil
}

// ProjectsWithoutDefaultEnvironment lists real projects that have no default environment, oldest first.
func (db *DB) ProjectsWithoutDefaultEnvironment(ctx context.Context) ([]Project, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.name, p.created_at, p.org_id FROM projects p
		WHERE p.id != ? AND NOT EXISTS (
			SELECT 1 FROM environments e WHERE e.project_id = p.id AND e.is_default = 1 AND e.scope = 'project')
		ORDER BY p.created_at
	`, GlobalProjectID)
	if err != nil {
		return nil, fmt.Errorf("store: projects without default environment: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Project
	for rows.Next() {
		var (
			p     Project
			orgID sql.NullString
		)
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &orgID); err != nil {
			return nil, fmt.Errorf("store: scan project: %w", err)
		}
		p.OrgID = orgID.String
		out = append(out, p)
	}
	return out, rows.Err()
}
