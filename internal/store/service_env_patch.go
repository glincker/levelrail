package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// PatchServiceEnv sets the given env keys on name and nothing else, reading
// and writing in one transaction so a concurrent edit is never reverted. A
// key is applied only while its stored value still equals expect[key] (absent
// equals absent), so a stale or hand-edited value is left alone. Returns the
// applied keys, or ErrServiceNotFound.
func (db *DB) PatchServiceEnv(ctx context.Context, name string, expect, set map[string]string) ([]string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: patch env for %q: begin transaction: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var raw string
	err = tx.QueryRowContext(ctx, `SELECT env FROM desired_services WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: patch env for %q: read: %w", name, err)
	}
	env := map[string]string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			return nil, fmt.Errorf("store: patch env for %q: decode: %w", name, err)
		}
	}
	var applied []string
	for k, v := range set {
		cur, has := env[k]
		old, hadOld := expect[k]
		if has != hadOld || cur != old {
			continue
		}
		env[k] = v
		applied = append(applied, k)
	}
	if len(applied) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("store: patch env for %q: encode: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET env = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(encoded), name); err != nil {
		return nil, fmt.Errorf("store: patch env for %q: write: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: patch env for %q: commit: %w", name, err)
	}
	sort.Strings(applied)
	return applied, nil
}
