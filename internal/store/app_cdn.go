package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetAppDNSProxied returns the per-app Cloudflare proxy override; set is false
// when the app inherits the global setting.
func (db *DB) GetAppDNSProxied(ctx context.Context, app string) (proxied, set bool, err error) {
	var v int
	err = db.QueryRowContext(ctx, `SELECT dns_proxied FROM app_cdn_settings WHERE app_name = ?`, app).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("store: get app dns proxied %s: %w", app, err)
	}
	return v != 0, true, nil
}

// SetAppDNSProxied stores the override, or clears it when proxied is nil.
func (db *DB) SetAppDNSProxied(ctx context.Context, app string, proxied *bool, actor string) error {
	if proxied == nil {
		if _, err := db.ExecContext(ctx, `DELETE FROM app_cdn_settings WHERE app_name = ?`, app); err != nil {
			return fmt.Errorf("store: clear app dns proxied %s: %w", app, err)
		}
		return nil
	}
	v := 0
	if *proxied {
		v = 1
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_cdn_settings (app_name, dns_proxied, updated_at, updated_by) VALUES (?, ?, ?, ?)
		ON CONFLICT (app_name) DO UPDATE SET dns_proxied = excluded.dns_proxied, updated_at = excluded.updated_at, updated_by = excluded.updated_by
	`, app, v, time.Now().UTC().Format(time.RFC3339), actor)
	if err != nil {
		return fmt.Errorf("store: set app dns proxied %s: %w", app, err)
	}
	return nil
}
