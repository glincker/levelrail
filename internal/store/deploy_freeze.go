package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// DeployFreezeScopeGlobal applies a freeze window to every app.
const DeployFreezeScopeGlobal = "global"

// DeployFreezeScopeApp returns the scope string for one app's own windows.
func DeployFreezeScopeApp(serviceName string) string { return "app:" + serviceName }

// DeployFreezeWindow is a recurring window, starting at each Cron match in
// Timezone and lasting Duration, during which automatic deploys are held.
type DeployFreezeWindow struct {
	ID        string
	Scope     string
	Cron      string
	Duration  time.Duration
	Timezone  string
	Reason    string
	CreatedAt time.Time
}

// ListDeployFreezeWindows returns the windows in any of scopes, oldest first.
func (db *DB) ListDeployFreezeWindows(ctx context.Context, scopes ...string) ([]DeployFreezeWindow, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(scopes))
	for _, s := range scopes {
		args = append(args, s)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, scope, cron, duration_seconds, timezone, reason, created_at
		FROM deploy_freeze_windows
		WHERE scope IN (?`+strings.Repeat(", ?", len(scopes)-1)+`)
		ORDER BY created_at ASC, id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy freeze windows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployFreezeWindow
	for rows.Next() {
		var (
			w         DeployFreezeWindow
			seconds   int64
			createdAt string
		)
		if err := rows.Scan(&w.ID, &w.Scope, &w.Cron, &seconds, &w.Timezone, &w.Reason, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan deploy freeze window: %w", err)
		}
		w.Duration = time.Duration(seconds) * time.Second
		if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
			w.CreatedAt = t
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate deploy freeze windows: %w", err)
	}
	return out, nil
}

// ReplaceDeployFreezeWindows makes windows the complete set for scope. IDs
// are minted here; an empty windows clears the scope.
func (db *DB) ReplaceDeployFreezeWindows(ctx context.Context, scope string, windows []DeployFreezeWindow) ([]DeployFreezeWindow, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: replace deploy freeze windows: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM deploy_freeze_windows WHERE scope = ?`, scope); err != nil {
		return nil, fmt.Errorf("store: replace deploy freeze windows: clear %q: %w", scope, err)
	}
	now := time.Now().UTC()
	out := make([]DeployFreezeWindow, 0, len(windows))
	for i, w := range windows {
		id, err := newFreezeWindowID()
		if err != nil {
			return nil, err
		}
		w.ID, w.Scope = id, scope
		w.CreatedAt = now.Add(time.Duration(i) * time.Microsecond)
		if w.Timezone == "" {
			w.Timezone = "UTC"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deploy_freeze_windows (id, scope, cron, duration_seconds, timezone, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, w.ID, w.Scope, w.Cron, int64(w.Duration/time.Second), w.Timezone, w.Reason, w.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return nil, fmt.Errorf("store: replace deploy freeze windows: insert: %w", err)
		}
		out = append(out, w)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: replace deploy freeze windows: commit: %w", err)
	}
	return out, nil
}

func newFreezeWindowID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate freeze window id: %w", err)
	}
	return "frz_" + hex.EncodeToString(buf), nil
}
