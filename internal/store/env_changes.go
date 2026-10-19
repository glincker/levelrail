package store

import (
	"context"
	"fmt"
	"time"
)

// Env change actions.
const (
	EnvChangeAdded   = "added"
	EnvChangeChanged = "changed"
	EnvChangeRemoved = "removed"
)

// envChangesKeepPerApp caps the history rows kept for one app.
const envChangesKeepPerApp = 1000

// EnvChange is one variable added, changed or removed on an app. It never carries a value.
type EnvChange struct {
	ID      int64
	AppName string
	At      time.Time
	Actor   string
	Key     string
	Action  string
	Source  string
}

// RecordEnvChanges appends changes for appName and trims the app's history to its cap.
func (db *DB) RecordEnvChanges(ctx context.Context, appName, actor, source string, changes []EnvChange) error {
	if len(changes) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: record env changes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO env_changes (app_name, at, actor, key, action, source) VALUES (?, ?, ?, ?, ?, ?)`,
			appName, at, actor, c.Key, c.Action, source); err != nil {
			return fmt.Errorf("store: record env changes: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM env_changes WHERE app_name = ? AND id NOT IN (SELECT id FROM env_changes WHERE app_name = ? ORDER BY id DESC LIMIT ?)`,
		appName, appName, envChangesKeepPerApp); err != nil {
		return fmt.Errorf("store: trim env changes: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: record env changes: %w", err)
	}
	return nil
}

// ListEnvChanges returns appName's history, newest first.
func (db *DB) ListEnvChanges(ctx context.Context, appName string, limit, offset int) ([]EnvChange, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, app_name, at, actor, key, action, source FROM env_changes WHERE app_name = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		appName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("store: list env changes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EnvChange
	for rows.Next() {
		var c EnvChange
		var at string
		if err := rows.Scan(&c.ID, &c.AppName, &at, &c.Actor, &c.Key, &c.Action, &c.Source); err != nil {
			return nil, fmt.Errorf("store: scan env change: %w", err)
		}
		c.At, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate env changes: %w", err)
	}
	return out, nil
}
