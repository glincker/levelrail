package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AppEnvSettings is an app's environment protection state (migrations/0486).
type AppEnvSettings struct {
	AppName string
	// SensitiveOverrides holds the operator's explicit per-variable choice;
	// a key absent here is classified automatically.
	SensitiveOverrides map[string]bool
	// SecretsWriteOnly blocks every reveal and full export for this app.
	SecretsWriteOnly bool
	UpdatedAt        time.Time
}

// GetAppEnvSettings returns name's settings, or the zero settings when none
// were saved. It never returns ErrServiceNotFound: callers already loaded the app.
func (db *DB) GetAppEnvSettings(ctx context.Context, name string) (AppEnvSettings, error) {
	var overridesJSON, updatedAt string
	var writeOnly int
	err := db.QueryRowContext(ctx, `SELECT sensitive_overrides, secrets_write_only, updated_at FROM app_env_settings WHERE app_name = ?`, name).
		Scan(&overridesJSON, &writeOnly, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AppEnvSettings{AppName: name}, nil
	}
	if err != nil {
		return AppEnvSettings{}, fmt.Errorf("store: get app env settings %q: %w", name, err)
	}
	return decodeAppEnvSettings(name, overridesJSON, writeOnly, updatedAt)
}

// ListAppEnvSettings returns every saved settings row keyed by app name.
func (db *DB) ListAppEnvSettings(ctx context.Context) (map[string]AppEnvSettings, error) {
	rows, err := db.QueryContext(ctx, `SELECT app_name, sensitive_overrides, secrets_write_only, updated_at FROM app_env_settings`)
	if err != nil {
		return nil, fmt.Errorf("store: list app env settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]AppEnvSettings{}
	for rows.Next() {
		var name, overridesJSON, updatedAt string
		var writeOnly int
		if err := rows.Scan(&name, &overridesJSON, &writeOnly, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: scan app env settings: %w", err)
		}
		s, err := decodeAppEnvSettings(name, overridesJSON, writeOnly, updatedAt)
		if err != nil {
			return nil, err
		}
		out[name] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list app env settings: %w", err)
	}
	return out, nil
}

func decodeAppEnvSettings(name, overridesJSON string, writeOnly int, updatedAt string) (AppEnvSettings, error) {
	s := AppEnvSettings{AppName: name, SecretsWriteOnly: writeOnly != 0}
	if err := json.Unmarshal([]byte(overridesJSON), &s.SensitiveOverrides); err != nil {
		return AppEnvSettings{}, fmt.Errorf("store: decode sensitive overrides for %q: %w", name, err)
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		s.UpdatedAt = t
	}
	return s, nil
}

// RemoveServicePlainEnvKey drops key from name's plain env without touching
// any other column and marks the env dirty. It reports whether key existed.
func (db *DB) RemoveServicePlainEnvKey(ctx context.Context, name, key string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: remove env key for %q: begin: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT env FROM desired_services WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrServiceNotFound
	}
	if err != nil {
		return false, fmt.Errorf("store: remove env key for %q: load: %w", name, err)
	}
	env := map[string]string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			return false, fmt.Errorf("store: remove env key for %q: decode: %w", name, err)
		}
	}
	if _, ok := env[key]; !ok {
		return false, nil
	}
	delete(env, key)
	out, err := json.Marshal(env)
	if err != nil {
		return false, fmt.Errorf("store: remove env key for %q: encode: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET env = ?, env_dirty = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, string(out), name); err != nil {
		return false, fmt.Errorf("store: remove env key for %q: update: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: remove env key for %q: commit: %w", name, err)
	}
	return true, nil
}

// SaveAppEnvSettings upserts s. The app must exist (foreign key).
func (db *DB) SaveAppEnvSettings(ctx context.Context, s AppEnvSettings) error {
	overrides := s.SensitiveOverrides
	if overrides == nil {
		overrides = map[string]bool{}
	}
	overridesJSON, err := json.Marshal(overrides)
	if err != nil {
		return fmt.Errorf("store: encode sensitive overrides for %q: %w", s.AppName, err)
	}
	writeOnly := 0
	if s.SecretsWriteOnly {
		writeOnly = 1
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO app_env_settings (app_name, sensitive_overrides, secrets_write_only, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (app_name) DO UPDATE SET
			sensitive_overrides = excluded.sensitive_overrides,
			secrets_write_only = excluded.secrets_write_only,
			updated_at = excluded.updated_at
	`, s.AppName, string(overridesJSON), writeOnly, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save app env settings %q: %w", s.AppName, err)
	}
	return nil
}
