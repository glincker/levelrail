package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetStaticSiteDeployHook returns the hash of the site's hook secret and when it was minted; empty when no hook exists.
func (db *DB) GetStaticSiteDeployHook(ctx context.Context, name string) (hash string, createdAt time.Time, err error) {
	var created string
	err = db.QueryRowContext(ctx, `SELECT deploy_hook_hash, deploy_hook_created_at FROM static_site_settings WHERE name = ?`, name).Scan(&hash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("store: get static site deploy hook %q: %w", name, err)
	}
	if created != "" {
		createdAt, _ = time.Parse(time.RFC3339Nano, created)
	}
	return hash, createdAt, nil
}

// SetStaticSiteDeployHook stores the secret hash; an empty hash revokes the hook.
func (db *DB) SetStaticSiteDeployHook(ctx context.Context, name, hash string, createdAt time.Time) error {
	created := ""
	if hash != "" {
		created = createdAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO static_site_settings (name, deploy_hook_hash, deploy_hook_created_at) VALUES (?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET deploy_hook_hash = excluded.deploy_hook_hash,
			deploy_hook_created_at = excluded.deploy_hook_created_at,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, name, hash, created)
	if err != nil {
		return fmt.Errorf("store: set static site deploy hook %q: %w", name, err)
	}
	return nil
}
