package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ErrEnvKeyIsSecret is returned when a merge tries to set a key the app already stores as a secret.
var ErrEnvKeyIsSecret = errors.New("store: env key is stored as a secret")

// EnvMergeResult is the outcome of MergeServiceEnv. Before and After are the
// whole plain env around the merge so callers can diff names; never log them.
type EnvMergeResult struct {
	Before  map[string]string
	After   map[string]string
	Changed bool
	// EnvDirty is the latch after the merge: true until a restart recreates the container.
	EnvDirty bool
}

// MergeServiceEnv applies set and unset to the plain env of one app inside a
// single BEGIN IMMEDIATE transaction and writes only env and env_dirty. It
// never rewrites the rest of the spec, so a concurrent full-spec writer cannot
// lose these keys and these keys cannot clobber that writer's fields.
func (db *DB) MergeServiceEnv(ctx context.Context, name string, set map[string]string, unset []string) (EnvMergeResult, error) {
	res, err := db.mergeServiceEnvTx(ctx, name, set, unset)
	if err != nil {
		return res, err
	}
	if res.Changed {
		if err := db.noteAppChange(ctx, name); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (db *DB) mergeServiceEnvTx(ctx context.Context, name string, set map[string]string, unset []string) (EnvMergeResult, error) {
	return db.mergeEnvCore(ctx, name, set, unset, false)
}

// ReplaceServiceEnv makes the plain env exactly env; the drop list is computed
// inside the write transaction so it matches the env the write sees.
func (db *DB) ReplaceServiceEnv(ctx context.Context, name string, env map[string]string) (EnvMergeResult, error) {
	res, err := db.mergeEnvCore(ctx, name, env, nil, true)
	if err != nil {
		return res, err
	}
	if res.Changed {
		if err := db.noteAppChange(ctx, name); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (db *DB) mergeEnvCore(ctx context.Context, name string, set map[string]string, unset []string, replace bool) (EnvMergeResult, error) {
	if err := checkEnvKeys(slices.Concat(mapKeys(set), unset)...); err != nil {
		return EnvMergeResult{}, err
	}
	// A dedicated connection lets us issue BEGIN IMMEDIATE, which takes the write lock up front.
	conn, err := db.Conn(ctx)
	if err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: %w", name, err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: begin: %w", name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()

	var envRaw, secretRaw string
	var dirty bool
	err = conn.QueryRowContext(ctx, `SELECT env, secret_env, env_dirty FROM desired_services WHERE name = ?`, name).Scan(&envRaw, &secretRaw, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return EnvMergeResult{}, ErrServiceNotFound
	}
	if err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: read: %w", name, err)
	}
	var current map[string]string
	if err := decodeJSONCol(envRaw, &current); err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: decode env: %w", name, err)
	}
	secrets, err := unmarshalSecretEnv(secretRaw)
	if err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: decode secret_env: %w", name, err)
	}
	secretNames := map[string]bool{}
	for _, n := range SecretEnvNames(secrets) {
		secretNames[n] = true
	}
	for k := range set {
		if secretNames[k] {
			return EnvMergeResult{}, fmt.Errorf("%w: %s", ErrEnvKeyIsSecret, k)
		}
	}

	if replace {
		for k := range current {
			if _, keep := set[k]; !keep {
				unset = append(unset, k)
			}
		}
	}
	next := maps.Clone(nonNilMap(current))
	for k, v := range set {
		next[k] = v
	}
	for _, k := range unset {
		delete(next, k)
	}
	res := EnvMergeResult{Before: nonNilMap(current), After: next, Changed: !maps.Equal(current, next)}
	res.EnvDirty = dirty || res.Changed
	if !res.Changed {
		committed = true
		_, err := conn.ExecContext(ctx, `COMMIT`)
		return res, err
	}
	nextJSON, err := json.Marshal(next)
	if err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: marshal: %w", name, err)
	}
	// env_dirty stays set until a restart recreates the container, the same latch the update endpoint keeps.
	if _, err := conn.ExecContext(ctx, `UPDATE desired_services SET env = ?, env_dirty = 1, env_version = env_version + 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(nextJSON), name); err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: write: %w", name, err)
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return EnvMergeResult{}, fmt.Errorf("store: merge env for %q: commit: %w", name, err)
	}
	committed = true
	return res, nil
}
