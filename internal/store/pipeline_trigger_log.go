package store

import (
	"context"
	"fmt"
	"time"
)

// PipelineTriggerLogKeep is how many trigger decisions are kept per app.
const PipelineTriggerLogKeep = 200

// Trigger decisions.
const (
	TriggerStarted = "started"
	TriggerHeld    = "held"
	TriggerSkipped = "skipped"
	TriggerFailed  = "failed"
)

// PipelineTriggerLog records why a git event did or did not start a run.
type PipelineTriggerLog struct {
	ID        int64
	AppName   string
	Pipeline  string
	Event     string
	Ref       string
	SHA       string
	Decision  string
	Reason    string
	RunID     string
	CreatedAt time.Time
}

// AddPipelineTriggerLog appends e and prunes the app's log to the newest
// PipelineTriggerLogKeep rows.
func (db *DB) AddPipelineTriggerLog(ctx context.Context, e PipelineTriggerLog) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO pipeline_trigger_log (app_name, pipeline_name, event, ref, sha, decision, reason, run_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, e.AppName, e.Pipeline, e.Event, e.Ref, e.SHA, e.Decision, e.Reason, e.RunID, formatTime(e.CreatedAt)); err != nil {
		return fmt.Errorf("store: add pipeline trigger log: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		DELETE FROM pipeline_trigger_log WHERE app_name = ? AND id NOT IN
			(SELECT id FROM pipeline_trigger_log WHERE app_name = ? ORDER BY id DESC LIMIT ?)
	`, e.AppName, e.AppName, PipelineTriggerLogKeep); err != nil {
		return fmt.Errorf("store: prune pipeline trigger log: %w", err)
	}
	return nil
}

// ListPipelineTriggerLog returns an app's trigger decisions newest first.
// limit <= 0 defaults to 50.
func (db *DB) ListPipelineTriggerLog(ctx context.Context, app string, limit int) ([]PipelineTriggerLog, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx, `
		SELECT id, app_name, pipeline_name, event, ref, sha, decision, reason, run_id, created_at
		FROM pipeline_trigger_log WHERE app_name = ? ORDER BY id DESC LIMIT ?
	`, app, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline trigger log: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineTriggerLog
	for rows.Next() {
		var e PipelineTriggerLog
		var at string
		if err := rows.Scan(&e.ID, &e.AppName, &e.Pipeline, &e.Event, &e.Ref, &e.SHA, &e.Decision, &e.Reason, &e.RunID, &at); err != nil {
			return nil, fmt.Errorf("store: scan pipeline trigger log: %w", err)
		}
		if e.CreatedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("store: parse trigger log time: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
