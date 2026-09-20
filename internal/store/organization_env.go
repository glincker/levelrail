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

	if _, err := tx.ExecContext(ctx, `DELETE FROM organization_env_vars WHERE org_id = ? AND is_secret = 0`, orgID); err != nil {
		return fmt.Errorf("store: set organization env vars for %q: clear existing: %w", orgID, err)
	}
	for key, value := range vars {
		// ON CONFLICT, not a plain INSERT: see SetProjectEnvVars' own doc
		// comment on why a same-named secret-marked row must survive the
		// DELETE above and be converted, not duplicated, here.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO organization_env_vars (org_id, key, value, is_secret, updated_at)
			VALUES (?, ?, ?, 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
			ON CONFLICT (org_id, key) DO UPDATE SET
				value = excluded.value,
				is_secret = 0,
				updated_at = excluded.updated_at
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
		SELECT key, value FROM organization_env_vars WHERE org_id = ? AND is_secret = 0
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
	orgID, err := db.GetProjectOrganizationID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("store: list organization env vars for project %q: %w", projectID, err)
	}
	if orgID == "" {
		return map[string]string{}, nil
	}
	return db.ListOrganizationEnvVars(ctx, orgID)
}

// GetProjectOrganizationID returns projectID's organization ID, or ""
// if the project has none (or doesn't exist): the same lookup
// ListOrganizationEnvVarsForProject already needed internally, exposed
// here for internal/sharedenv.Resolver, which needs the ID itself (to
// resolve that organization's secret-marked shared env vars) rather
// than just its plain vars.
func (db *DB) GetProjectOrganizationID(ctx context.Context, projectID string) (string, error) {
	var orgID sql.NullString
	err := db.QueryRowContext(ctx, `SELECT org_id FROM projects WHERE id = ?`, projectID).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get organization id for project %q: %w", projectID, err)
	}
	return orgID.String, nil
}

// OrganizationEnvSecretsKey is the internal/secrets serviceName an
// organization's secret-marked shared env vars are stored under,
// mirroring ProjectEnvSecretsKey one tier up.
func OrganizationEnvSecretsKey(orgID string) string {
	return "organization-env/" + orgID
}

// SetOrganizationSecretEnvVar marks key as a secret-backed shared env
// var for orgID, mirroring SetProjectSecretEnvVar one tier up.
func (db *DB) SetOrganizationSecretEnvVar(ctx context.Context, orgID, key string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO organization_env_vars (org_id, key, value, is_secret, updated_at)
		VALUES (?, ?, '', 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		ON CONFLICT (org_id, key) DO UPDATE SET
			value = '',
			is_secret = 1,
			updated_at = excluded.updated_at
	`, orgID, key)
	if err != nil {
		return fmt.Errorf("store: set organization secret env var for %q/%q: %w", orgID, key, err)
	}
	return nil
}

// DeleteOrganizationSecretEnvVar removes key's secret-marked row for
// orgID, mirroring DeleteProjectSecretEnvVar one tier up.
func (db *DB) DeleteOrganizationSecretEnvVar(ctx context.Context, orgID, key string) error {
	if _, err := db.ExecContext(ctx, `
		DELETE FROM organization_env_vars WHERE org_id = ? AND key = ? AND is_secret = 1
	`, orgID, key); err != nil {
		return fmt.Errorf("store: delete organization secret env var for %q/%q: %w", orgID, key, err)
	}
	return nil
}

// ListOrganizationSecretEnvKeys returns every secret-marked shared env
// var key for orgID, never a value, mirroring ListProjectSecretEnvKeys
// one tier up.
func (db *DB) ListOrganizationSecretEnvKeys(ctx context.Context, orgID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key FROM organization_env_vars WHERE org_id = ? AND is_secret = 1 ORDER BY key
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list organization secret env keys for %q: %w", orgID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("store: scan organization secret env key for %q: %w", orgID, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate organization secret env keys for %q: %w", orgID, err)
	}
	return keys, nil
}

// ListOrganizationEnvVarsDetailed returns every shared env var for
// orgID, plain and secret-marked alike, mirroring
// ListProjectEnvVarsDetailed one tier up.
func (db *DB) ListOrganizationEnvVarsDetailed(ctx context.Context, orgID string) ([]SharedEnvVar, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value, is_secret FROM organization_env_vars WHERE org_id = ? ORDER BY key
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("store: list detailed organization env vars for %q: %w", orgID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []SharedEnvVar
	for rows.Next() {
		var v SharedEnvVar
		var isSecret int
		if err := rows.Scan(&v.Key, &v.Value, &isSecret); err != nil {
			return nil, fmt.Errorf("store: scan detailed organization env var for %q: %w", orgID, err)
		}
		v.Secret = isSecret != 0
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate detailed organization env vars for %q: %w", orgID, err)
	}
	return out, nil
}
