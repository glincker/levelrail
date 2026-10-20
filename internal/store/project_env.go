package store

import (
	"context"
	"fmt"
	"time"
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM project_env_vars WHERE project_id = ? AND is_secret = 0`, projectID); err != nil {
		return fmt.Errorf("store: set project env vars for %q: clear existing: %w", projectID, err)
	}
	for key, value := range vars {
		// ON CONFLICT, not a plain INSERT: the same key may already exist
		// as a secret-marked row (SetProjectSecretEnvVar), which the
		// DELETE above deliberately left alone. Setting it here through
		// this plain, non-secret path converts it back to plain, the
		// same "last write wins, no separate lock" semantics shared env
		// vars have always had.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO project_env_vars (project_id, key, value, is_secret, updated_at)
			VALUES (?, ?, ?, 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT (project_id, key) DO UPDATE SET
				value = excluded.value,
				is_secret = 0,
				updated_at = excluded.updated_at
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
		SELECT key, value FROM project_env_vars WHERE project_id = ? AND is_secret = 0
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

// ProjectEnvSecretsKey is the internal/secrets serviceName a project's
// secret-marked shared env vars are stored under, distinct from the
// plain app-runtime service namespace, the same
// distinct-namespace-from-the-real-service reasoning GitSourceSecretsKey
// already establishes.
func ProjectEnvSecretsKey(projectID string) string {
	return "project-env/" + projectID
}

// SetProjectSecretEnvVar marks key as a secret-backed shared env var for
// projectID, upserting a placeholder row (is_secret = 1, value = ”).
// The plaintext itself is never passed here: callers encrypt it
// separately via internal/secrets.Manager.SetValue under
// ProjectEnvSecretsKey(projectID), the same split SaveServiceDEK/
// SaveSecretValue already keep from this table's own row. Unlike
// SetProjectEnvVars this is a single-key upsert, not a full replace, so
// it can never clobber the project's other vars, secret or not.
func (db *DB) SetProjectSecretEnvVar(ctx context.Context, projectID, key string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO project_env_vars (project_id, key, value, is_secret, updated_at)
		VALUES (?, ?, '', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (project_id, key) DO UPDATE SET
			value = '',
			is_secret = 1,
			updated_at = excluded.updated_at
	`, projectID, key)
	if err != nil {
		return fmt.Errorf("store: set project secret env var for %q/%q: %w", projectID, key, err)
	}
	return nil
}

// DeleteProjectSecretEnvVar removes key's secret-marked row for
// projectID. Idempotent: a key with no such row is not an error. The
// underlying ciphertext in service_secret_values (if any) is left in
// place, unreachable once this row is gone: the same orphaned-ciphertext
// tolerance DeleteServiceSecrets' own doc comment already accepts.
func (db *DB) DeleteProjectSecretEnvVar(ctx context.Context, projectID, key string) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM project_env_vars WHERE project_id = ? AND key = ? AND is_secret = 1
	`, projectID, key); err != nil {
		return fmt.Errorf("store: delete project secret env var for %q/%q: %w", projectID, key, err)
	}
	return nil
}

// ListProjectSecretEnvKeys returns every secret-marked shared env var
// key for projectID, never a value, mirroring ListSecretKeys' own
// "names only" shape for per-app secrets.
func (db *DB) ListProjectSecretEnvKeys(ctx context.Context, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key FROM project_env_vars WHERE project_id = ? AND is_secret = 1 ORDER BY key
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list project secret env keys for %q: %w", projectID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("store: scan project secret env key for %q: %w", projectID, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate project secret env keys for %q: %w", projectID, err)
	}
	return keys, nil
}

// ListProjectEnvVarsDetailed returns every shared env var for projectID,
// plain and secret-marked alike, secret entries carrying an empty Value
// (see SharedEnvVar's own doc comment): the shape GET
// /api/v1/projects/{id}/env/all (project_env.go) needs to render one
// combined settings-page table instead of stitching two separate list
// calls together itself.
func (db *DB) ListProjectEnvVarsDetailed(ctx context.Context, projectID string) ([]SharedEnvVar, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value, is_secret, updated_at FROM project_env_vars WHERE project_id = ? ORDER BY key
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list detailed project env vars for %q: %w", projectID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []SharedEnvVar
	for rows.Next() {
		var v SharedEnvVar
		var isSecret int
		var updatedAtRaw string
		if err := rows.Scan(&v.Key, &v.Value, &isSecret, &updatedAtRaw); err != nil {
			return nil, fmt.Errorf("store: scan detailed project env var for %q: %w", projectID, err)
		}
		v.Secret = isSecret != 0
		updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("store: parse project env var updated_at for %q/%q: %w", projectID, v.Key, err)
		}
		v.UpdatedAt = updatedAt
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate detailed project env vars for %q: %w", projectID, err)
	}
	return out, nil
}
