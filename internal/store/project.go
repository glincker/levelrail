package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrProjectNotFound is returned by GetProject and DeleteProject when id
// doesn't match any row.
var ErrProjectNotFound = errors.New("store: project not found")

// Project is a lightweight, non-auth organizational grouping (see
// migrations/0022_projects.sql's own comment for the "why" and its
// explicit boundary against the deferred teams/RBAC work): a name an
// app or database can optionally be filed under, nothing more. There is
// no owner, no member list, no permission of any kind attached to a
// project; the single admin user sees every project exactly the way it
// already sees every app and database.
type Project struct {
	ID        string
	Name      string
	CreatedAt string
	OrgID     string
}

// SaveProject inserts a new project row. IDs are minted by the caller
// (internal/api's randomProjectID), the same "generate before the
// INSERT" pattern SaveBackupTarget already establishes, and this is
// insert-only, never an upsert: a project's name is a pure label with
// no addressing role (migrations/0022_projects.sql's own comment on why
// it isn't UNIQUE), so there is no legitimate "same id, replace
// contents" case the way SaveDesiredService's full-replace semantics
// exist for.
func (db *DB) SaveProject(ctx context.Context, p Project) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO projects (id, name, created_at)
		VALUES (?, ?, ?)
	`, p.ID, p.Name, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save project %q: %w", p.ID, err)
	}
	return nil
}

// GetProject returns the project with this ID, or ErrProjectNotFound.
func (db *DB) GetProject(ctx context.Context, id string) (Project, error) {
	var (
		p     Project
		orgID sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT id, name, created_at, org_id
		FROM projects
		WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.CreatedAt, &orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrProjectNotFound
	}
	if err != nil {
		return Project{}, fmt.Errorf("store: get project %q: %w", id, err)
	}
	p.OrgID = orgID.String
	return p, nil
}

// ListProjects returns every project, oldest first, the same creation-
// order convention ListBackupTargets already uses for the same reason:
// it's the order a settings-style listing page wants, and there is no
// other field (like an app/database's name) a caller would obviously
// want to sort by instead.
func (db *DB) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, created_at, org_id
		FROM projects
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list projects: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []Project
	for rows.Next() {
		var (
			p     Project
			orgID sql.NullString
		)
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &orgID); err != nil {
			return nil, fmt.Errorf("store: scan project row: %w", err)
		}
		p.OrgID = orgID.String
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate project rows: %w", err)
	}
	return out, nil
}

// DeleteProject removes a project row. Unlike DeleteBackupTarget (which
// relies on foreign key enforcement to *reject* a delete while
// backup_history still references the target), desired_services.
// project_id and desired_databases.project_id are declared
// "REFERENCES projects(id) ON DELETE SET NULL"
// (migrations/0022_projects.sql), so this delete always succeeds and
// every app/database that belonged to this project is left in place,
// simply project-less again (project_id reverts to NULL), the same
// real, valid, permanent state an app/database that was never assigned
// to a project is already in. There is deliberately no cascade here:
// a project is an organizational label, not a lifecycle owner, so
// deleting the label must never delete the real resources it labeled.
// Returns ErrProjectNotFound if id doesn't exist.
func (db *DB) DeleteProject(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete project %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete project %q: %w", id, err)
	}
	if n == 0 {
		return ErrProjectNotFound
	}
	return nil
}
