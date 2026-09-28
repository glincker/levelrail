package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Pipeline run and job statuses. Terminal statuses never transition again.
const (
	PipelineStatusQueued          = "queued"
	PipelineStatusPending         = "pending"
	PipelineStatusRunning         = "running"
	PipelineStatusWaitingApproval = "waiting_approval"
	PipelineStatusSucceeded       = "succeeded"
	PipelineStatusFailed          = "failed"
	PipelineStatusCancelled       = "cancelled"
	PipelineStatusSkipped         = "skipped"
)

// IsPipelineTerminal reports whether status is a final state.
func IsPipelineTerminal(status string) bool {
	switch status {
	case PipelineStatusSucceeded, PipelineStatusFailed, PipelineStatusCancelled, PipelineStatusSkipped:
		return true
	}
	return false
}

var (
	// ErrPipelineNotFound is returned when a pipeline ID or name matches no row.
	ErrPipelineNotFound = errors.New("store: pipeline not found")
	// ErrPipelineRunNotFound is returned when a run ID matches no row.
	ErrPipelineRunNotFound = errors.New("store: pipeline run not found")
)

// Pipeline is one saved pipeline definition scoped to an app.
type Pipeline struct {
	ID        string
	AppName   string
	Name      string
	Source    string
	YAML      string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
	// SourceSHA is the commit a repo-sourced pipeline was synced from.
	SourceSHA string
	// SyncedHash is the SHA-256 of the YAML as synced; a different hash of
	// the current YAML means the definition was edited afterwards.
	SyncedHash string
}

const pipelineCols = `id, app_name, name, source, yaml, enabled, created_at, updated_at, source_sha, synced_hash`

func scanPipeline(scan func(...any) error) (Pipeline, error) {
	var p Pipeline
	var created, updated string
	if err := scan(&p.ID, &p.AppName, &p.Name, &p.Source, &p.YAML, &p.Enabled, &created, &updated, &p.SourceSHA, &p.SyncedHash); err != nil {
		return Pipeline{}, err
	}
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Pipeline{}, fmt.Errorf("parse created_at: %w", err)
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return Pipeline{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return p, nil
}

// SavePipeline inserts p, or replaces the YAML, enabled flag, and sync
// bookkeeping of the existing pipeline with the same app and name. The stored row is returned.
func (db *DB) SavePipeline(ctx context.Context, p Pipeline) (Pipeline, error) {
	if p.Source == "" {
		p.Source = "ui"
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO pipelines (id, app_name, name, source, yaml, enabled, created_at, updated_at, source_sha, synced_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (app_name, name) DO UPDATE SET
			source = excluded.source, yaml = excluded.yaml, enabled = excluded.enabled, updated_at = excluded.updated_at,
			source_sha = excluded.source_sha, synced_hash = excluded.synced_hash
	`, p.ID, p.AppName, p.Name, p.Source, p.YAML, p.Enabled, formatTime(p.CreatedAt), formatTime(p.UpdatedAt), p.SourceSHA, p.SyncedHash)
	if err != nil {
		return Pipeline{}, fmt.Errorf("store: save pipeline %q: %w", p.Name, err)
	}
	return db.GetPipelineByName(ctx, p.AppName, p.Name)
}

// GetPipeline returns the pipeline with this ID, or ErrPipelineNotFound.
func (db *DB) GetPipeline(ctx context.Context, id string) (Pipeline, error) {
	p, err := scanPipeline(db.QueryRowContext(ctx, `SELECT `+pipelineCols+` FROM pipelines WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Pipeline{}, ErrPipelineNotFound
	}
	if err != nil {
		return Pipeline{}, fmt.Errorf("store: get pipeline %q: %w", id, err)
	}
	return p, nil
}

// GetPipelineByName returns an app's pipeline by name, or ErrPipelineNotFound.
func (db *DB) GetPipelineByName(ctx context.Context, app, name string) (Pipeline, error) {
	p, err := scanPipeline(db.QueryRowContext(ctx, `SELECT `+pipelineCols+` FROM pipelines WHERE app_name = ? AND name = ?`, app, name).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Pipeline{}, ErrPipelineNotFound
	}
	if err != nil {
		return Pipeline{}, fmt.Errorf("store: get pipeline %q/%q: %w", app, name, err)
	}
	return p, nil
}

// ListPipelines returns an app's pipelines by name; an empty app lists all.
func (db *DB) ListPipelines(ctx context.Context, app string) ([]Pipeline, error) {
	q := `SELECT ` + pipelineCols + ` FROM pipelines`
	var args []any
	if app != "" {
		q += ` WHERE app_name = ?`
		args = append(args, app)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY app_name, name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list pipelines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Pipeline
	for rows.Next() {
		p, err := scanPipeline(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan pipeline: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePipeline removes a pipeline and, by cascade, its runs.
func (db *DB) DeletePipeline(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM pipelines WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete pipeline %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPipelineNotFound
	}
	return nil
}

// SetPipelineEnabled flips a pipeline's enabled flag.
func (db *DB) SetPipelineEnabled(ctx context.Context, id string, enabled bool, now time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE pipelines SET enabled = ?, updated_at = ? WHERE id = ?`, enabled, formatTime(now), id)
	if err != nil {
		return fmt.Errorf("store: set pipeline %q enabled: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPipelineNotFound
	}
	return nil
}
