package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// AppContextConfig is one app's overrides for one deploy context
// (migrations/2650_app_context_config.sql). Env holds plain values only.
type AppContextConfig struct {
	AppName      string
	Context      string
	Env          map[string]string
	BuildCommand string
	PublishDir   string
	BuildArgs    map[string]string
	UpdatedAt    string
}

// SaveAppContextConfig upserts the overrides for c.Context.
func (db *DB) SaveAppContextConfig(ctx context.Context, c AppContextConfig) error {
	env, err := json.Marshal(nonNilMap(c.Env))
	if err != nil {
		return fmt.Errorf("store: encode context env for %q/%q: %w", c.AppName, c.Context, err)
	}
	args, err := json.Marshal(nonNilMap(c.BuildArgs))
	if err != nil {
		return fmt.Errorf("store: encode context build args for %q/%q: %w", c.AppName, c.Context, err)
	}
	if c.UpdatedAt == "" {
		c.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO app_context_config (app_name, context, env, build_command, publish_dir, build_args, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name, context) DO UPDATE SET env = excluded.env, build_command = excluded.build_command,
			publish_dir = excluded.publish_dir, build_args = excluded.build_args, updated_at = excluded.updated_at
	`, c.AppName, c.Context, string(env), c.BuildCommand, c.PublishDir, string(args), c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save context config for %q/%q: %w", c.AppName, c.Context, err)
	}
	return nil
}

// DeleteAppContextConfig removes one context's overrides and reports whether a row existed.
func (db *DB) DeleteAppContextConfig(ctx context.Context, appName, contextName string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM app_context_config WHERE app_name = ? AND context = ?`, appName, contextName)
	if err != nil {
		return false, fmt.Errorf("store: delete context config for %q/%q: %w", appName, contextName, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: delete context config for %q/%q: rows affected: %w", appName, contextName, err)
	}
	return n > 0, nil
}

// ListAppContextConfigs returns every context's overrides for appName, ordered by context.
func (db *DB) ListAppContextConfigs(ctx context.Context, appName string) ([]AppContextConfig, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT app_name, context, env, build_command, publish_dir, build_args, updated_at
		FROM app_context_config WHERE app_name = ? ORDER BY context
	`, appName)
	if err != nil {
		return nil, fmt.Errorf("store: list context configs for %q: %w", appName, err)
	}
	defer func() { _ = rows.Close() }()

	var out []AppContextConfig
	for rows.Next() {
		var c AppContextConfig
		var env, args string
		if err := rows.Scan(&c.AppName, &c.Context, &env, &c.BuildCommand, &c.PublishDir, &args, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan context config for %q: %w", appName, err)
		}
		if err := json.Unmarshal([]byte(env), &c.Env); err != nil {
			return nil, fmt.Errorf("store: decode context env for %q/%q: %w", appName, c.Context, err)
		}
		if err := json.Unmarshal([]byte(args), &c.BuildArgs); err != nil {
			return nil, fmt.Errorf("store: decode context build args for %q/%q: %w", appName, c.Context, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate context configs for %q: %w", appName, err)
	}
	return out, nil
}
