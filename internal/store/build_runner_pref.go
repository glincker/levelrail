package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// BuildRunnerPref is an app's preferred or pinned build runner.
type BuildRunnerPref struct {
	App    string
	NodeID string
	Pinned bool
	// Labels are "key" or "key=value" requirements every runner for this app must carry.
	Labels []string
}

// GetBuildRunnerPref returns the app's runner preference, false when none is set.
func (db *DB) GetBuildRunnerPref(ctx context.Context, app string) (BuildRunnerPref, bool, error) {
	p := BuildRunnerPref{App: app}
	var pinned int
	var labels string
	err := db.QueryRowContext(ctx, `SELECT node_id, pinned, labels FROM app_build_runner WHERE app_name = ?`, app).Scan(&p.NodeID, &pinned, &labels)
	if errors.Is(err, sql.ErrNoRows) {
		return BuildRunnerPref{}, false, nil
	}
	if err != nil {
		return BuildRunnerPref{}, false, fmt.Errorf("store: get build runner pref: %w", err)
	}
	p.Pinned = pinned == 1
	if labels != "" {
		p.Labels = strings.Split(labels, ",")
	}
	return p, true, nil
}

// SetBuildRunnerPref upserts an app's runner preference.
func (db *DB) SetBuildRunnerPref(ctx context.Context, p BuildRunnerPref, now time.Time) error {
	pinned := 0
	if p.Pinned {
		pinned = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO app_build_runner (app_name, node_id, pinned, labels, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET node_id = excluded.node_id, pinned = excluded.pinned, labels = excluded.labels, updated_at = excluded.updated_at`,
		p.App, p.NodeID, pinned, strings.Join(p.Labels, ","), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set build runner pref: %w", err)
	}
	return nil
}

// ClearBuildRunnerPref removes an app's runner preference.
func (db *DB) ClearBuildRunnerPref(ctx context.Context, app string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_build_runner WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: clear build runner pref: %w", err)
	}
	return nil
}
