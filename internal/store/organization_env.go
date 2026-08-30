package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SetOrganizationEnvVars full-replaces orgID's shared env vars with vars,
// the same "replace, don't diff" semantics SetProjectEnvVars already
// establishes one tier down.
func (db *DB) SetOrganizationEnvVars(ctx context.Context, orgID string, vars map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set organization env vars for %q: begin transaction: %w", orgID, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM organization_env_vars WHERE org_id = ?`, orgID); err != nil {
		return fmt.Errorf("store: set organization env vars for %q: clear existing: %w", orgID, err)
	}
	for key, value := range vars {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO organization_env_vars (org_id, key, value, updated_at)
			VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		`, orgID, key, value); err != nil {
			return fmt.Errorf("store: set organization env vars for %q: insert %q: %w", orgID, key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set organization env vars for %q: commit: %w", orgID, err)
	}
	return nil
}

// ListOrganizationEnvVars returns orgID's shared env vars as a plain map,
// empty (not nil) when none are set, mirroring ListProjectEnvVars.
func (db *DB) ListOrganizationEnvVars(ctx context.Context, orgID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value FROM organization_env_vars WHERE org_id = ?
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list organization env vars for %q: %w", orgID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("store: scan organization env var for %q: %w", orgID, err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate organization env vars for %q: %w", orgID, err)
	}
	return out, nil
}

// ListOrganizationEnvVarsForProject returns the shared env vars of
// projectID's organization, empty when the project has no organization
// (or doesn't exist): the join
// internal/reconcile/application.Controller.resolveEnv needs to add the
// organization tier below its existing project tier without that
// package having to know projects carry an org_id column at all.
func (db *DB) ListOrganizationEnvVarsForProject(ctx context.Context, projectID string) (map[string]string, error) {
	var orgID sql.NullString
	err := db.QueryRowContext(ctx, `SELECT org_id FROM projects WHERE id = ?`, projectID).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: list organization env vars for project %q: lookup org: %w", projectID, err)
	}
	if !orgID.Valid || orgID.String == "" {
		return map[string]string{}, nil
	}
	return db.ListOrganizationEnvVars(ctx, orgID.String)
}
