package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// GetAppCacheEpoch returns the cache token of the app instance, or "" when none exists yet.
func (db *DB) GetAppCacheEpoch(ctx context.Context, name string) (string, error) {
	var epoch string
	err := db.QueryRowContext(ctx, `SELECT epoch FROM app_cache_epochs WHERE name = ?`, name).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get app cache epoch %q: %w", name, err)
	}
	return epoch, nil
}

// EnsureAppCacheEpoch returns the cache token of the app instance, minting one on first use.
func (db *DB) EnsureAppCacheEpoch(ctx context.Context, name string) (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("store: mint app cache epoch: %w", err)
	}
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO app_cache_epochs (name, epoch, created_at) VALUES (?, ?, ?)`,
		name, hex.EncodeToString(raw[:]), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return "", fmt.Errorf("store: ensure app cache epoch %q: %w", name, err)
	}
	return db.GetAppCacheEpoch(ctx, name)
}

// DeleteAppCacheEpoch ends the instance: the next app of this name starts a cold cache.
func (db *DB) DeleteAppCacheEpoch(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_cache_epochs WHERE name = ?`, name); err != nil {
		return fmt.Errorf("store: delete app cache epoch %q: %w", name, err)
	}
	return nil
}
