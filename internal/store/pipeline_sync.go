package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PipelineSync is an app's repository sync settings and last outcome. The
// zero value (no row) means repo_is_truth off and never synced.
type PipelineSync struct {
	AppName     string
	RepoIsTruth bool
	LastSHA     string
	LastSyncAt  time.Time
	LastError   string
}

// GetPipelineSync returns the sync state for app, or a zero-valued state
// carrying only the app name when none has been recorded.
func (db *DB) GetPipelineSync(ctx context.Context, app string) (PipelineSync, error) {
	out := PipelineSync{AppName: app}
	var at string
	err := db.QueryRowContext(ctx, `SELECT repo_is_truth, last_sha, last_synced_at, last_error FROM pipeline_sync WHERE app_name = ?`, app).
		Scan(&out.RepoIsTruth, &out.LastSHA, &at, &out.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return PipelineSync{}, fmt.Errorf("store: get pipeline sync %q: %w", app, err)
	}
	if at != "" {
		if out.LastSyncAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return PipelineSync{}, fmt.Errorf("store: parse pipeline sync time: %w", err)
		}
	}
	return out, nil
}

// SavePipelineSync upserts the sync state for s.AppName.
func (db *DB) SavePipelineSync(ctx context.Context, s PipelineSync) error {
	at := ""
	if !s.LastSyncAt.IsZero() {
		at = formatTime(s.LastSyncAt)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO pipeline_sync (app_name, repo_is_truth, last_sha, last_synced_at, last_error)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (app_name) DO UPDATE SET
			repo_is_truth = excluded.repo_is_truth, last_sha = excluded.last_sha,
			last_synced_at = excluded.last_synced_at, last_error = excluded.last_error
	`, s.AppName, s.RepoIsTruth, s.LastSHA, at, s.LastError)
	if err != nil {
		return fmt.Errorf("store: save pipeline sync %q: %w", s.AppName, err)
	}
	return nil
}
