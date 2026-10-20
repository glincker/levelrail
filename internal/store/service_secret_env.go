package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// SetServiceSecretEnvDeclared adds (declared) or removes key from name's
// secret-backed env names without touching any other column, and reports
// whether the set changed. An existing entry keeps its Required flag.
func (db *DB) SetServiceSecretEnvDeclared(ctx context.Context, name, key string, declared bool) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: declare secret env for %q: begin: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var raw string
	err = tx.QueryRowContext(ctx, `SELECT secret_env FROM desired_services WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrServiceNotFound
	}
	if err != nil {
		return false, fmt.Errorf("store: declare secret env for %q: load: %w", name, err)
	}
	refs, err := unmarshalSecretEnv(raw)
	if err != nil {
		return false, fmt.Errorf("store: declare secret env for %q: decode: %w", name, err)
	}

	idx := -1
	for i, ref := range refs {
		if ref.Name == key {
			idx = i
			break
		}
	}
	switch {
	case declared && idx >= 0, !declared && idx < 0:
		return false, nil
	case declared:
		refs = append(refs, SecretEnvRef{Name: key})
	default:
		refs = append(refs[:idx], refs[idx+1:]...)
	}

	out, err := json.Marshal(nonNilSecretEnv(refs))
	if err != nil {
		return false, fmt.Errorf("store: declare secret env for %q: marshal: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET secret_env = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(out), name); err != nil {
		return false, fmt.Errorf("store: declare secret env for %q: update: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: declare secret env for %q: commit: %w", name, err)
	}
	return true, nil
}

// DeleteSecretValue removes one stored secret value, reporting whether a row
// existed. The service's DEK and other values are untouched.
func (db *DB) DeleteSecretValue(ctx context.Context, serviceName, envKey string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM service_secret_values WHERE service_name = ? AND env_key = ?`, serviceName, envKey)
	if err != nil {
		return false, fmt.Errorf("store: delete secret value %q/%q: %w", serviceName, envKey, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: delete secret value %q/%q: rows affected: %w", serviceName, envKey, err)
	}
	return n > 0, nil
}
