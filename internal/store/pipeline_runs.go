package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PipelineRun is one execution of a pipeline. Definition is the YAML
// snapshot the run started with.
type PipelineRun struct {
	ID               string
	PipelineID       string
	AppName          string
	Number           int
	TriggerKind      string
	TriggerActor     string
	Ref              string
	CommitSHA        string
	InputsJSON       string
	Definition       string
	ConcurrencyGroup string
	CancelInProgress bool
	Status           string
	Reason           string
	CancelRequested  bool
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

const pipelineRunCols = `id, pipeline_id, app_name, number, trigger_kind, trigger_actor, ref, commit_sha, inputs, definition,
	concurrency_group, cancel_in_progress, status, reason, cancel_requested, created_at, started_at, finished_at`

func scanPipelineRun(scan func(...any) error) (PipelineRun, error) {
	var r PipelineRun
	var created string
	var started, finished sql.NullString
	if err := scan(&r.ID, &r.PipelineID, &r.AppName, &r.Number, &r.TriggerKind, &r.TriggerActor, &r.Ref, &r.CommitSHA,
		&r.InputsJSON, &r.Definition, &r.ConcurrencyGroup, &r.CancelInProgress, &r.Status, &r.Reason, &r.CancelRequested,
		&created, &started, &finished); err != nil {
		return PipelineRun{}, err
	}
	var err error
	if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return PipelineRun{}, fmt.Errorf("parse created_at: %w", err)
	}
	if r.StartedAt, err = parseTimePtr(started); err != nil {
		return PipelineRun{}, fmt.Errorf("parse started_at: %w", err)
	}
	if r.FinishedAt, err = parseTimePtr(finished); err != nil {
		return PipelineRun{}, fmt.Errorf("parse finished_at: %w", err)
	}
	return r, nil
}

// CreatePipelineRun inserts r as queued and assigns its per-pipeline number.
func (db *DB) CreatePipelineRun(ctx context.Context, r PipelineRun) (PipelineRun, error) {
	if r.InputsJSON == "" {
		r.InputsJSON = "{}"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return PipelineRun{}, fmt.Errorf("store: create pipeline run: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number), 0) + 1 FROM pipeline_runs WHERE pipeline_id = ?`, r.PipelineID).Scan(&r.Number); err != nil {
		return PipelineRun{}, fmt.Errorf("store: next pipeline run number: %w", err)
	}
	r.Status = PipelineStatusQueued
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO pipeline_runs (id, pipeline_id, app_name, number, trigger_kind, trigger_actor, ref, commit_sha, inputs, definition,
			concurrency_group, cancel_in_progress, status, reason, cancel_requested, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
	`, r.ID, r.PipelineID, r.AppName, r.Number, r.TriggerKind, r.TriggerActor, r.Ref, r.CommitSHA, r.InputsJSON, r.Definition,
		r.ConcurrencyGroup, r.CancelInProgress, r.Status, r.Reason, formatTime(r.CreatedAt)); err != nil {
		return PipelineRun{}, fmt.Errorf("store: insert pipeline run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return PipelineRun{}, fmt.Errorf("store: commit pipeline run: %w", err)
	}
	return r, nil
}

// GetPipelineRun returns a run by ID, or ErrPipelineRunNotFound.
func (db *DB) GetPipelineRun(ctx context.Context, id string) (PipelineRun, error) {
	r, err := scanPipelineRun(db.QueryRowContext(ctx, `SELECT `+pipelineRunCols+` FROM pipeline_runs WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PipelineRun{}, ErrPipelineRunNotFound
	}
	if err != nil {
		return PipelineRun{}, fmt.Errorf("store: get pipeline run %q: %w", id, err)
	}
	return r, nil
}

func (db *DB) queryPipelineRuns(ctx context.Context, where string, args ...any) ([]PipelineRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+pipelineRunCols+` FROM pipeline_runs `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query pipeline runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineRun
	for rows.Next() {
		r, err := scanPipelineRun(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan pipeline run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListPipelineRuns returns an app's runs newest first, optionally limited
// to one pipeline. limit <= 0 defaults to 50.
func (db *DB) ListPipelineRuns(ctx context.Context, app, pipelineID string, limit int) ([]PipelineRun, error) {
	if limit <= 0 {
		limit = 50
	}
	where := `WHERE app_name = ?`
	args := []any{app}
	if pipelineID != "" {
		where += ` AND pipeline_id = ?`
		args = append(args, pipelineID)
	}
	return db.queryPipelineRuns(ctx, where+` ORDER BY created_at DESC, number DESC LIMIT ?`, append(args, limit)...)
}

// ListActivePipelineRuns returns every queued or running run, oldest first.
func (db *DB) ListActivePipelineRuns(ctx context.Context) ([]PipelineRun, error) {
	return db.queryPipelineRuns(ctx, `WHERE status IN ('queued', 'running') ORDER BY created_at, number`)
}

// ListPipelineRunsInGroup returns active runs sharing a concurrency group.
func (db *DB) ListPipelineRunsInGroup(ctx context.Context, group string) ([]PipelineRun, error) {
	return db.queryPipelineRuns(ctx, `WHERE concurrency_group = ? AND status IN ('queued', 'running') ORDER BY created_at, number`, group)
}

// SetPipelineRunStatus moves a run to status with a reason. started and
// finished, when non-nil, are recorded; existing values are kept otherwise.
func (db *DB) SetPipelineRunStatus(ctx context.Context, id, status, reason string, started, finished *time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE pipeline_runs SET status = ?, reason = ?,
			started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at)
		WHERE id = ?
	`, status, reason, formatTimePtr(started), formatTimePtr(finished), id)
	if err != nil {
		return fmt.Errorf("store: set pipeline run %q status: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPipelineRunNotFound
	}
	return nil
}

// RequestPipelineRunCancel flags a non-terminal run for cancellation; the
// executor observes the flag on its next pass.
func (db *DB) RequestPipelineRunCancel(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `UPDATE pipeline_runs SET cancel_requested = 1 WHERE id = ? AND status IN ('queued', 'running')`, id)
	if err != nil {
		return fmt.Errorf("store: cancel pipeline run %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := db.GetPipelineRun(ctx, id); gerr != nil {
			return gerr
		}
	}
	return nil
}

// PrunePipelineRuns deletes finished runs beyond the newest keep per
// pipeline and returns how many were removed.
func (db *DB) PrunePipelineRuns(ctx context.Context, keep int) (int64, error) {
	res, err := db.ExecContext(ctx, `
		DELETE FROM pipeline_runs WHERE status IN ('succeeded', 'failed', 'cancelled') AND id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY pipeline_id ORDER BY number DESC) AS rn
				FROM pipeline_runs WHERE status IN ('succeeded', 'failed', 'cancelled')
			) WHERE rn > ?
		)
	`, keep)
	if err != nil {
		return 0, fmt.Errorf("store: prune pipeline runs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
