package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"
)

const prEnvOverrideIDPrefix = "prenv_"

// ErrPRPreviewEnvOverrideNotFound means no override row matches the id.
var ErrPRPreviewEnvOverrideNotFound = errors.New("store: pr preview env override not found")

// PRPreviewEnvOverride is one env var override for a single pull request's
// preview. Value is always "" when Secret is true.
type PRPreviewEnvOverride struct {
	ID          string
	ServiceName string
	PRNumber    int
	Key         string
	Value       string
	Secret      bool
	UpdatedAt   time.Time
}

// PRPreviewEnvSecretsKey is the secrets namespace holding a secret override's
// ciphertext, one per (service, pull request) so teardown can drop it whole.
func PRPreviewEnvSecretsKey(serviceName string, prNumber int) string {
	return "pr-env/" + serviceName + "/" + strconv.Itoa(prNumber)
}

// SetPRPreviewEnvOverride upserts one override keyed on (service, pr, key) and
// returns its id. The caller encrypts a secret's real value separately.
func (db *DB) SetPRPreviewEnvOverride(ctx context.Context, serviceName string, prNumber int, key, value string, secret bool) (string, error) {
	if secret {
		value = ""
	}
	var id string
	err := db.QueryRowContext(ctx, `
		SELECT id FROM preview_pr_env_overrides WHERE service_name = ? AND pr_number = ? AND key = ?
	`, serviceName, prNumber, key).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		buf := make([]byte, 9)
		if _, rerr := rand.Read(buf); rerr != nil {
			return "", fmt.Errorf("store: set pr env override: generate id: %w", rerr)
		}
		id = prEnvOverrideIDPrefix + base64.RawURLEncoding.EncodeToString(buf)
		if _, err := db.ExecContext(ctx, `
			INSERT INTO preview_pr_env_overrides (id, service_name, pr_number, key, value, is_secret, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		`, id, serviceName, prNumber, key, value, boolToInt(secret)); err != nil {
			return "", fmt.Errorf("store: set pr env override %q/%d/%q: insert: %w", serviceName, prNumber, key, err)
		}
	case err != nil:
		return "", fmt.Errorf("store: set pr env override %q/%d/%q: read existing: %w", serviceName, prNumber, key, err)
	default:
		if _, err := db.ExecContext(ctx, `
			UPDATE preview_pr_env_overrides SET value = ?, is_secret = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?
		`, value, boolToInt(secret), id); err != nil {
			return "", fmt.Errorf("store: set pr env override %q/%d/%q: update: %w", serviceName, prNumber, key, err)
		}
	}
	return id, nil
}

// ListPRPreviewEnvOverrides returns one pull request's overrides ordered by key.
func (db *DB) ListPRPreviewEnvOverrides(ctx context.Context, serviceName string, prNumber int) ([]PRPreviewEnvOverride, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, pr_number, key, value, is_secret, updated_at
		FROM preview_pr_env_overrides WHERE service_name = ? AND pr_number = ? ORDER BY key
	`, serviceName, prNumber)
	if err != nil {
		return nil, fmt.Errorf("store: list pr env overrides for %q/%d: %w", serviceName, prNumber, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PRPreviewEnvOverride
	for rows.Next() {
		var (
			o        PRPreviewEnvOverride
			isSecret int
			raw      string
		)
		if err := rows.Scan(&o.ID, &o.ServiceName, &o.PRNumber, &o.Key, &o.Value, &isSecret, &raw); err != nil {
			return nil, fmt.Errorf("store: scan pr env override for %q/%d: %w", serviceName, prNumber, err)
		}
		o.Secret = isSecret != 0
		if o.UpdatedAt, err = time.Parse(time.RFC3339Nano, raw); err != nil {
			return nil, fmt.Errorf("store: parse pr env override updated_at for %q/%d/%q: %w", serviceName, prNumber, o.Key, err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate pr env overrides for %q/%d: %w", serviceName, prNumber, err)
	}
	return out, nil
}

// DeletePRPreviewEnvOverride removes one override, scoped to the service and
// pull request so an id from another app or PR cannot be deleted by guessing.
func (db *DB) DeletePRPreviewEnvOverride(ctx context.Context, serviceName string, prNumber int, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM preview_pr_env_overrides WHERE id = ? AND service_name = ? AND pr_number = ?`, id, serviceName, prNumber)
	if err != nil {
		return fmt.Errorf("store: delete pr env override %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete pr env override %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrPRPreviewEnvOverrideNotFound
	}
	return nil
}

// DeleteAllPRPreviewEnvOverrides drops every override of one pull request.
// Deleting nothing is success, so a retried teardown converges.
func (db *DB) DeleteAllPRPreviewEnvOverrides(ctx context.Context, serviceName string, prNumber int) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM preview_pr_env_overrides WHERE service_name = ? AND pr_number = ?`, serviceName, prNumber); err != nil {
		return fmt.Errorf("store: delete pr env overrides for %q/%d: %w", serviceName, prNumber, err)
	}
	return nil
}

// AddPreviewPendingVolumes records volumes a preview teardown must still
// remove. Re-adding a name is a no-op.
func (db *DB) AddPreviewPendingVolumes(ctx context.Context, previewAppID string, names []string) error {
	for _, n := range names {
		if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO preview_pending_volumes (preview_app_id, volume_name) VALUES (?, ?)`, previewAppID, n); err != nil {
			return fmt.Errorf("store: add pending preview volume %q for %q: %w", n, previewAppID, err)
		}
	}
	return nil
}

// ListPreviewPendingVolumes returns the volumes still pending removal.
func (db *DB) ListPreviewPendingVolumes(ctx context.Context, previewAppID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT volume_name FROM preview_pending_volumes WHERE preview_app_id = ? ORDER BY volume_name`, previewAppID)
	if err != nil {
		return nil, fmt.Errorf("store: list pending preview volumes for %q: %w", previewAppID, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("store: scan pending preview volume for %q: %w", previewAppID, err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ClearPreviewPendingVolumes forgets a preview's pending volumes once removed.
func (db *DB) ClearPreviewPendingVolumes(ctx context.Context, previewAppID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM preview_pending_volumes WHERE preview_app_id = ?`, previewAppID); err != nil {
		return fmt.Errorf("store: clear pending preview volumes for %q: %w", previewAppID, err)
	}
	return nil
}
