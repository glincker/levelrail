package store

import (
	"context"
	"fmt"
	"time"
)

// RuntimeSettingRow is one saved runtime setting; absence means env or default applies.
type RuntimeSettingRow struct {
	Key       string
	Value     string
	Actor     string
	UpdatedAt time.Time
}

// ListRuntimeSettings returns every saved runtime setting row.
func (db *DB) ListRuntimeSettings(ctx context.Context) ([]RuntimeSettingRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value, actor, updated_at FROM runtime_settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("store: list runtime settings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []RuntimeSettingRow
	for rows.Next() {
		var r RuntimeSettingRow
		var at string
		if err := rows.Scan(&r.Key, &r.Value, &r.Actor, &at); err != nil {
			return nil, fmt.Errorf("store: scan runtime setting: %w", err)
		}
		r.UpdatedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list runtime settings: %w", err)
	}
	return out, nil
}

// SetRuntimeSetting saves a runtime setting value, replacing any previous one.
func (db *DB) SetRuntimeSetting(ctx context.Context, key, value, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO runtime_settings (key, value, actor, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, actor = excluded.actor, updated_at = excluded.updated_at`,
		key, value, actor, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set runtime setting %s: %w", key, err)
	}
	return nil
}

// DeleteRuntimeSetting removes a saved value so env or the default applies again.
func (db *DB) DeleteRuntimeSetting(ctx context.Context, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM runtime_settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("store: delete runtime setting %s: %w", key, err)
	}
	return nil
}
