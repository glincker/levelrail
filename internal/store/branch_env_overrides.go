package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// branchEnvOverrideIDPrefix mirrors previewEnvironmentIDPrefix's own
// scheme: a short, greppable prefix on an otherwise opaque random ID.
const branchEnvOverrideIDPrefix = "benv_"

// NewBranchEnvOverrideID mints a random branch env override ID, the
// same crypto/rand-plus-base64 scheme NewPreviewEnvironmentID already
// establishes.
func NewBranchEnvOverrideID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate branch env override id: %w", err)
	}
	return branchEnvOverrideIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// ErrBranchEnvOverrideNotFound means no row exists for the id a caller
// asked to delete.
var ErrBranchEnvOverrideNotFound = errors.New("store: branch env override not found")

// ServiceBranchEnvOverride is one branch-scoped env var override row
// (migrations/0119): serviceName's env var key resolves to Value (or,
// if Secret, a decrypted value under BranchEnvOverrideSecretsKey)
// whenever a preview built from BranchPattern is deployed, in place of
// whatever serviceName's own Env/SecretEnv/VaultEnv holds for that key.
type ServiceBranchEnvOverride struct {
	ID            string
	ServiceName   string
	BranchPattern string
	Key           string
	// Value is always "" when Secret is true, matching SharedEnvVar's
	// own "never carry a secret's plaintext on this struct" rule.
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

// BranchEnvOverrideSecretsKey is the internal/secrets serviceName a
// branch-scoped override's secret-marked value is stored under,
// mirroring EnvironmentEnvSecretsKey: one DEK per (service, branch
// pattern) pair, independent of serviceName's own per-app secrets, so
// deleting a branch override never touches any other stored secret.
func BranchEnvOverrideSecretsKey(serviceName, branchPattern string) string {
	return "branch-env/" + serviceName + "/" + branchPattern
}

// SetServiceBranchEnvOverride upserts one branch-scoped env var
// override for serviceName, keyed on (serviceName, branchPattern, key).
// value is ignored (stored as "") when secret is true: the caller
// (internal/api) is responsible for encrypting the real value via
// internal/secrets.Manager under BranchEnvOverrideSecretsKey before or
// after this call, the same split SetEnvironmentSecretEnvVar and
// handleSetSharedEnvSecret already establish between "mark secret" and
// "store ciphertext." Returns the row's id, minting a new one only on
// first insert; an existing row keeps its own id across updates.
func (db *DB) SetServiceBranchEnvOverride(ctx context.Context, serviceName, branchPattern, key, value string, secret bool) (string, error) {
	if secret {
		value = ""
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("store: set branch env override for %q/%q/%q: begin transaction: %w", serviceName, branchPattern, key, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM service_branch_env_overrides WHERE service_name = ? AND branch_pattern = ? AND key = ?
	`, serviceName, branchPattern, key).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id, err = NewBranchEnvOverrideID()
		if err != nil {
			return "", fmt.Errorf("store: set branch env override for %q/%q/%q: %w", serviceName, branchPattern, key, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO service_branch_env_overrides (id, service_name, branch_pattern, key, value, is_secret, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		`, id, serviceName, branchPattern, key, value, boolToInt(secret)); err != nil {
			return "", fmt.Errorf("store: set branch env override for %q/%q/%q: insert: %w", serviceName, branchPattern, key, err)
		}
	case err != nil:
		return "", fmt.Errorf("store: set branch env override for %q/%q/%q: read existing: %w", serviceName, branchPattern, key, err)
	default:
		if _, err := tx.ExecContext(ctx, `
			UPDATE service_branch_env_overrides
			SET value = ?, is_secret = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
			WHERE id = ?
		`, value, boolToInt(secret), id); err != nil {
			return "", fmt.Errorf("store: set branch env override for %q/%q/%q: update: %w", serviceName, branchPattern, key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("store: set branch env override for %q/%q/%q: commit: %w", serviceName, branchPattern, key, err)
	}
	return id, nil
}

// DeleteServiceBranchEnvOverride removes one branch-scoped override by
// id, scoped to serviceName so a caller authorized for one app's own
// resource (IAM's appResourceFromPath) can never delete another app's
// override by guessing or reusing its id. Only the row itself is
// removed: any ciphertext this override's key held under
// BranchEnvOverrideSecretsKey is left in place, unreachable once the
// row is gone, the same orphaned-ciphertext tolerance
// handleDeleteSharedEnvSecret's own doc comment already accepts.
// Returns ErrBranchEnvOverrideNotFound if id doesn't exist under
// serviceName.
func (db *DB) DeleteServiceBranchEnvOverride(ctx context.Context, serviceName, id string) error {
	result, err := db.ExecContext(ctx, `DELETE FROM service_branch_env_overrides WHERE id = ? AND service_name = ?`, id, serviceName)
	if err != nil {
		return fmt.Errorf("store: delete branch env override %q: %w", id, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete branch env override %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrBranchEnvOverrideNotFound
	}
	return nil
}

// ListServiceBranchEnvOverrides returns every branch-scoped override
// declared for serviceName, across every branch pattern, ordered by
// branch pattern then key for a stable listing.
func (db *DB) ListServiceBranchEnvOverrides(ctx context.Context, serviceName string) ([]ServiceBranchEnvOverride, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, branch_pattern, key, value, is_secret, updated_at
		FROM service_branch_env_overrides
		WHERE service_name = ?
		ORDER BY branch_pattern, key
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list branch env overrides for %q: %w", serviceName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ServiceBranchEnvOverride
	for rows.Next() {
		var (
			o            ServiceBranchEnvOverride
			isSecret     int
			updatedAtRaw string
		)
		if err := rows.Scan(&o.ID, &o.ServiceName, &o.BranchPattern, &o.Key, &o.Value, &isSecret, &updatedAtRaw); err != nil {
			return nil, fmt.Errorf("store: scan branch env override for %q: %w", serviceName, err)
		}
		o.Secret = isSecret != 0
		updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtRaw)
		if err != nil {
			return nil, fmt.Errorf("store: parse branch env override updated_at for %q/%q: %w", serviceName, o.Key, err)
		}
		o.UpdatedAt = updatedAt
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate branch env overrides for %q: %w", serviceName, err)
	}
	return out, nil
}
