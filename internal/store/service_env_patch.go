package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// PatchServiceEnv sets the given env keys on name and nothing else, reading
// and writing in one transaction so a concurrent domain edit or deploy is
// never reverted by a stale snapshot. Returns ErrServiceNotFound.
func (db *DB) PatchServiceEnv(ctx context.Context, name string, set map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: patch env for %q: begin transaction: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var raw string
	err = tx.QueryRowContext(ctx, `SELECT env FROM desired_services WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: patch env for %q: read: %w", name, err)
	}
	env := map[string]string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			return fmt.Errorf("store: patch env for %q: decode: %w", name, err)
		}
	}
	for k, v := range set {
		env[k] = v
	}
	encoded, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("store: patch env for %q: encode: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET env = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(encoded), name); err != nil {
		return fmt.Errorf("store: patch env for %q: write: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: patch env for %q: commit: %w", name, err)
	}
	return nil
}
