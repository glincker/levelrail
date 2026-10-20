package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// SetServicePreviewEnvOverride sets or clears one preview-specific env
// var override on name's own desired state (migrations/0098), the only
// way preview_env_overrides ever changes: SaveDesiredService's own doc
// comment explains why this is deliberately excluded from that method's
// full-record-replace semantics, the same reasoning
// SetServiceVaultEnvVar already establishes for vault_env. value nil
// clears the override.
func (db *DB) SetServicePreviewEnvOverride(ctx context.Context, name, envVar string, value *string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set preview env override for service %q: begin transaction: %w", name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	var overridesJSON string
	err = tx.QueryRowContext(ctx, `SELECT preview_env_overrides FROM desired_services WHERE name = ?`, name).Scan(&overridesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: set preview env override for service %q: read existing: %w", name, err)
	}

	overrides := map[string]string{}
	if overridesJSON != "" {
		if err := json.Unmarshal([]byte(overridesJSON), &overrides); err != nil {
			return fmt.Errorf("store: set preview env override for service %q: decode existing: %w", name, err)
		}
	}
	if value == nil {
		delete(overrides, envVar)
	} else {
		overrides[envVar] = *value
	}

	updated, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("store: set preview env override for service %q: encode: %w", name, err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE desired_services SET preview_env_overrides = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, string(updated), name); err != nil {
		return fmt.Errorf("store: set preview env override for service %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set preview env override for service %q: commit: %w", name, err)
	}
	return nil
}
