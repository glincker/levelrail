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

	if _, err := tx.ExecContext(ctx, `DELETE FROM environment_env_vars WHERE environment_id = ? AND is_secret = 0`, environmentID); err != nil {
		return fmt.Errorf("store: set environment env vars for %q: clear existing: %w", environmentID, err)
	}
	for key, value := range vars {
		// ON CONFLICT, not a plain INSERT: see SetProjectEnvVars' own doc
		// comment on why a same-named secret-marked row must survive the
		// DELETE above and be converted, not duplicated, here.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO environment_env_vars (environment_id, key, value, is_secret, updated_at)
			VALUES (?, ?, ?, 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT (environment_id, key) DO UPDATE SET
				value = excluded.value,
				is_secret = 0,
				updated_at = excluded.updated_at
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
		SELECT key, value FROM environment_env_vars WHERE environment_id = ? AND is_secret = 0
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

// EnvironmentEnvSecretsKey is the internal/secrets serviceName an
// environment's secret-marked shared env vars are stored under,
// mirroring ProjectEnvSecretsKey/OrganizationEnvSecretsKey.
func EnvironmentEnvSecretsKey(environmentID string) string {
	return "environment-env/" + environmentID
}

// SetEnvironmentSecretEnvVar marks key as a secret-backed shared env
// var for environmentID, mirroring SetProjectSecretEnvVar.
func (db *DB) SetEnvironmentSecretEnvVar(ctx context.Context, environmentID, key string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO environment_env_vars (environment_id, key, value, is_secret, updated_at)
		VALUES (?, ?, '', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (environment_id, key) DO UPDATE SET
			value = '',
			is_secret = 1,
			updated_at = excluded.updated_at
	`, environmentID, key)
	if err != nil {
		return fmt.Errorf("store: set environment secret env var for %q/%q: %w", environmentID, key, err)
	}
	return nil
}

// DeleteEnvironmentSecretEnvVar removes key's secret-marked row for
// environmentID, mirroring DeleteProjectSecretEnvVar.
func (db *DB) DeleteEnvironmentSecretEnvVar(ctx context.Context, environmentID, key string) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM environment_env_vars WHERE environment_id = ? AND key = ? AND is_secret = 1
	`, environmentID, key); err != nil {
		return fmt.Errorf("store: delete environment secret env var for %q/%q: %w", environmentID, key, err)
	}
	return nil
}

// ListEnvironmentSecretEnvKeys returns every secret-marked shared env
// var key for environmentID, never a value, mirroring
// ListProjectSecretEnvKeys.
func (db *DB) ListEnvironmentSecretEnvKeys(ctx context.Context, environmentID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key FROM environment_env_vars WHERE environment_id = ? AND is_secret = 1 ORDER BY key
	`, environmentID)
	if err != nil {
		return nil, fmt.Errorf("store: list environment secret env keys for %q: %w", environmentID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("store: scan environment secret env key for %q: %w", environmentID, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate environment secret env keys for %q: %w", environmentID, err)
	}
	return keys, nil
}

// ListEnvironmentEnvVarsDetailed returns every shared env var for
// environmentID, plain and secret-marked alike, mirroring
// ListProjectEnvVarsDetailed.
func (db *DB) ListEnvironmentEnvVarsDetailed(ctx context.Context, environmentID string) ([]SharedEnvVar, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value, is_secret FROM environment_env_vars WHERE environment_id = ? ORDER BY key
	`, environmentID)
	if err != nil {
		return nil, fmt.Errorf("store: list detailed environment env vars for %q: %w", environmentID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []SharedEnvVar
	for rows.Next() {
		var v SharedEnvVar
		var isSecret int
		if err := rows.Scan(&v.Key, &v.Value, &isSecret); err != nil {
			return nil, fmt.Errorf("store: scan detailed environment env var for %q: %w", environmentID, err)
		}
		v.Secret = isSecret != 0
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate detailed environment env vars for %q: %w", environmentID, err)
	}
	return out, nil
}
