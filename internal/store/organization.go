package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrOrganizationNotFound is returned by GetOrganization and
// DeleteOrganization when id doesn't match any row.
var ErrOrganizationNotFound = errors.New("store: organization not found")

// Organization groups projects (migrations/0054, mirroring Project's own
// opaque-id, no-auth-attached shape). Deleting an organization leaves
// its projects in place, org-less again (ON DELETE SET NULL).
type Organization struct {
	ID        string
	Name      string
	CreatedAt string
}

// SaveOrganization inserts a new organization row, insert-only like
// SaveProject.
func (db *DB) SaveOrganization(ctx context.Context, o Organization) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO organizations (id, name, created_at)
		VALUES (?, ?, ?)
	`, o.ID, o.Name, o.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save organization %q: %w", o.ID, err)
	}
	return nil
}

// GetOrganization returns the organization with this ID, or
// ErrOrganizationNotFound.
func (db *DB) GetOrganization(ctx context.Context, id string) (Organization, error) {
	var o Organization
	err := db.QueryRowContext(ctx, `
		SELECT id, name, created_at
		FROM organizations
		WHERE id = ?
	`, id).Scan(&o.ID, &o.Name, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Organization{}, ErrOrganizationNotFound
	}
	if err != nil {
		return Organization{}, fmt.Errorf("store: get organization %q: %w", id, err)
	}
	return o, nil
}

// ListOrganizations returns every organization, oldest first.
func (db *DB) ListOrganizations(ctx context.Context) ([]Organization, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, created_at
		FROM organizations
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list organizations: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []Organization
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan organization row: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate organization rows: %w", err)
	}
	return out, nil
}

// DeleteOrganization removes an organization row. projects.org_id is
// ON DELETE SET NULL (migrations/0054), so every project in this
// organization is left running, simply org-less again.
func (db *DB) DeleteOrganization(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM organizations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete organization %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete organization %q: %w", id, err)
	}
	if n == 0 {
		return ErrOrganizationNotFound
	}
	return nil
}

// SetProjectOrganization assigns or clears (orgID == "") a project's
// organization. Returns ErrProjectNotFound if the project doesn't exist.
func (db *DB) SetProjectOrganization(ctx context.Context, projectID, orgID string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE projects SET org_id = ? WHERE id = ?
	`, sql.NullString{String: orgID, Valid: orgID != ""}, projectID)
	if err != nil {
		return fmt.Errorf("store: set project %q organization: %w", projectID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: set project %q organization: %w", projectID, err)
	}
	if n == 0 {
		return ErrProjectNotFound
	}
	return nil
}
