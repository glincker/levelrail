package store

import (
	"context"
	"fmt"
)

// SetProjectEnvVars full-replaces projectID's shared env vars with
// vars: every key not present in vars is removed, every key in vars is
// written, in one transaction, the same "replace, don't diff" shape
// SaveDesiredService's own claimServiceDomains uses for a service's
// domain set. There is no partial-update path; a caller wanting to add
// one key reads the current set first (ListProjectEnvVars) and sends
// the whole merged map back.
func (db *DB) SetProjectEnvVars(ctx context.Context, projectID string, vars map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set project env vars for %q: begin transaction: %w", projectID, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM project_env_vars WHERE project_id = ?`, projectID); err != nil {
		return fmt.Errorf("store: set project env vars for %q: clear existing: %w", projectID, err)
	}
	for key, value := range vars {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_env_vars (project_id, key, value, updated_at)
			VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		`, projectID, key, value); err != nil {
			return fmt.Errorf("store: set project env vars for %q: insert %q: %w", projectID, key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set project env vars for %q: commit: %w", projectID, err)
	}
	return nil
}

// ListProjectEnvVars returns projectID's shared env vars as a plain map,
// empty (not nil) when none are set: the one caller that matters,
// internal/reconcile/application.Controller.resolveEnv, merges this
// straight into a working map with no nil check needed.
func (db *DB) ListProjectEnvVars(ctx context.Context, projectID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value FROM project_env_vars WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list project env vars for %q: %w", projectID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("store: scan project env var for %q: %w", projectID, err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate project env vars for %q: %w", projectID, err)
	}
	return out, nil
}
