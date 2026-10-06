package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
	// Kind, Scope and SortOrder come from migrations/0375: Kind is one of
	// the EnvironmentKind* constants, Scope is project or global.
	Kind      string
	Scope     string
	SortOrder int
}

// Environment kinds, Environment.Kind's valid values.
const (
	EnvironmentKindDev        = "dev"
	EnvironmentKindTest       = "test"
	EnvironmentKindUAT        = "uat"
	EnvironmentKindProduction = "production"
	EnvironmentKindPreview    = "preview"
	EnvironmentKindCustom     = "custom"
)

// Environment scopes, Environment.Scope's valid values.
const (
	EnvironmentScopeProject = "project"
	EnvironmentScopeGlobal  = "global"
)

const environmentColumns = "id, project_id, name, protected, created_at, kind, scope, sort_order"

func scanEnvironment(scan func(dest ...any) error) (Environment, error) {
	var e Environment
	err := scan(&e.ID, &e.ProjectID, &e.Name, &e.Protected, &e.CreatedAt, &e.Kind, &e.Scope, &e.SortOrder)
	return e, err
}

// SaveEnvironment inserts a new environment row, insert-only.
func (db *DB) SaveEnvironment(ctx context.Context, e Environment) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO environments (id, project_id, name, protected, created_at, kind, scope, sort_order)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, e.ID, e.ProjectID, e.Name, e.Protected, e.CreatedAt, kindOrCustom(e.Kind), scopeOrProject(e.Scope), e.SortOrder)
	if err != nil {
		return fmt.Errorf("store: save environment %q: %w", e.ID, err)
	}
	return nil
}

// GetEnvironment returns the environment with this ID, or
// ErrEnvironmentNotFound.
func (db *DB) GetEnvironment(ctx context.Context, id string) (Environment, error) {
	e, err := scanEnvironment(db.QueryRowContext(ctx, `
		SELECT `+environmentColumns+`
		FROM environments
		WHERE id = ?
	`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Environment{}, ErrEnvironmentNotFound
	}
	if err != nil {
		return Environment{}, fmt.Errorf("store: get environment %q: %w", id, err)
	}
	return e, nil
}

// GetEnvironmentsByIDs returns every environment in ids, keyed by ID, in
// one query (json_each, matching GetConditionsForControllers' pattern)
// instead of a GetEnvironment call per ID. An ID with no matching row is
// simply absent from the map. Built for GET /api/v1/apps (internal/api's
// environmentNames), which previously called GetEnvironment once per
// distinct environment on the page.
func (db *DB) GetEnvironmentsByIDs(ctx context.Context, ids []string) (map[string]Environment, error) {
	result := make(map[string]Environment, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("store: marshal batched environment ids: %w", err)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT `+environmentColumns+`
		FROM environments
		WHERE id IN (SELECT value FROM json_each(?))
	`, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("store: get environments for %d ids: %w", len(ids), err)
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		e, err := scanEnvironment(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan batched environment row: %w", err)
		}
		result[e.ID] = e
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate batched environment rows: %w", err)
	}
	return result, nil
}

// ListEnvironmentsByProject returns every environment for one project,
// oldest first.
func (db *DB) ListEnvironmentsByProject(ctx context.Context, projectID string) ([]Environment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+environmentColumns+`
		FROM environments
		WHERE project_id = ?
		ORDER BY sort_order, created_at
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list environments for project %q: %w", projectID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []Environment
	for rows.Next() {
		e, err := scanEnvironment(rows.Scan)
		if err != nil {
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

func kindOrCustom(k string) string {
	if k == "" {
		return EnvironmentKindCustom
	}
	return k
}

func scopeOrProject(s string) string {
	if s == "" {
		return EnvironmentScopeProject
	}
	return s
}
