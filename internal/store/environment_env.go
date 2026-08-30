package store

import (
	"context"
	"fmt"
)

// SetEnvironmentEnvVars full-replaces environmentID's shared env vars
// with vars, the same "replace, don't diff" semantics
// SetOrganizationEnvVars/SetProjectEnvVars already establish.
func (db *DB) SetEnvironmentEnvVars(ctx context.Context, environmentID string, vars map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set environment env vars for %q: begin transaction: %w", environmentID, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM environment_env_vars WHERE environment_id = ?`, environmentID); err != nil {
		return fmt.Errorf("store: set environment env vars for %q: clear existing: %w", environmentID, err)
	}
	for key, value := range vars {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO environment_env_vars (environment_id, key, value, updated_at)
			VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		`, environmentID, key, value); err != nil {
			return fmt.Errorf("store: set environment env vars for %q: insert %q: %w", environmentID, key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set environment env vars for %q: commit: %w", environmentID, err)
	}
	return nil
}

// ListEnvironmentEnvVars returns environmentID's shared env vars as a
// plain map, empty (not nil) when none are set, mirroring
// ListOrganizationEnvVars/ListProjectEnvVars.
func (db *DB) ListEnvironmentEnvVars(ctx context.Context, environmentID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value FROM environment_env_vars WHERE environment_id = ?
	`, environmentID)
	if err != nil {
		return nil, fmt.Errorf("store: list environment env vars for %q: %w", environmentID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("store: scan environment env var for %q: %w", environmentID, err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate environment env vars for %q: %w", environmentID, err)
	}
	return out, nil
}
