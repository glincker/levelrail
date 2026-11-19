package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetStaticSitePasswordHash returns the site's bcrypt gate hash, empty when it is open.
func (db *DB) GetStaticSitePasswordHash(ctx context.Context, name string) (string, error) {
	var h string
	err := db.QueryRowContext(ctx, `SELECT password_hash FROM static_site_settings WHERE name = ?`, name).Scan(&h)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("store: get static site gate %q: %w", name, err)
	}
	return h, nil
}

// SetStaticSitePasswordHash stores the bcrypt hash; an empty hash opens the site.
func (db *DB) SetStaticSitePasswordHash(ctx context.Context, name, hash string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO static_site_settings (name, password_hash) VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET password_hash = excluded.password_hash, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	`, name, hash)
	if err != nil {
		return fmt.Errorf("store: set static site gate %q: %w", name, err)
	}
	return nil
}

// ListStaticSitePasswordHashes returns the gate hash of every protected site, keyed by name.
func (db *DB) ListStaticSitePasswordHashes(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name, password_hash FROM static_site_settings WHERE password_hash != ''`)
	if err != nil {
		return nil, fmt.Errorf("store: list static site gates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var n, h string
		if err := rows.Scan(&n, &h); err != nil {
			return nil, fmt.Errorf("store: scan static site gate: %w", err)
		}
		out[n] = h
	}
	return out, rows.Err()
}
