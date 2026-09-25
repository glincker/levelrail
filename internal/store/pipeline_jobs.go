package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// PipelineJob is one expanded job of a run. NeedsJSON and MatrixJSON are
// JSON-encoded (a string list and a string map).
type PipelineJob struct {
	ID          string
	RunID       string
	Key         string
	DisplayName string
	Stage       string
	NeedsJSON   string
	MatrixJSON  string
	NodeID      string
	Status      string
	Reason      string
	Attempt     int
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Steps       []PipelineStep
}

// PipelineStep is one step row of a job.
type PipelineStep struct {
	JobID      string
	Index      int
	Name       string
	Kind       string
	Status     string
	Reason     string
	ExitCode   *int
	Attempt    int
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// PipelineLogLine is one captured output line.
type PipelineLogLine struct {
	ID        int64
	RunID     string
	JobKey    string
	StepIndex int
	Stream    string
	Line      string
	CreatedAt time.Time
}

// PipelineApproval is one manual approval gate.
type PipelineApproval struct {
	ID              int64
	RunID           string
	JobID           string
	StepIndex       int
	Message         string
	RequiredAbility string
	Decision        string
	DecidedBy       string
	Comment         string
	ExpiresAt       *time.Time
	CreatedAt       time.Time
	DecidedAt       *time.Time
}

// CreatePipelineJobs inserts jobs and their steps for a run in one transaction.
func (db *DB) CreatePipelineJobs(ctx context.Context, jobs []PipelineJob) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create pipeline jobs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, j := range jobs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pipeline_jobs (id, run_id, job_key, display_name, stage, needs, matrix, node_id, status, reason, attempt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
		`, j.ID, j.RunID, j.Key, j.DisplayName, j.Stage, j.NeedsJSON, j.MatrixJSON, j.NodeID, j.Status, j.Reason); err != nil {
			return fmt.Errorf("store: insert pipeline job %q: %w", j.Key, err)
		}
		for _, s := range j.Steps {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO pipeline_steps (job_id, idx, name, kind, status, reason) VALUES (?, ?, ?, ?, ?, '')
			`, j.ID, s.Index, s.Name, s.Kind, PipelineStatusPending); err != nil {
				return fmt.Errorf("store: insert pipeline step %q/%d: %w", j.Key, s.Index, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit pipeline jobs: %w", err)
	}
	return nil
}

// ListPipelineJobs returns a run's jobs with steps, in creation order.
func (db *DB) ListPipelineJobs(ctx context.Context, runID string) ([]PipelineJob, error) {
	jobs, err := db.readPipelineJobs(ctx, runID)
	if err != nil {
		return nil, err
	}
	steps, err := db.readPipelineSteps(ctx, runID)
	if err != nil {
		return nil, err
	}
	byJob := make(map[string]int, len(jobs))
	for i, j := range jobs {
		byJob[j.ID] = i
	}
	for _, s := range steps {
		if i, ok := byJob[s.JobID]; ok {
			jobs[i].Steps = append(jobs[i].Steps, s)
		}
	}
	return jobs, nil
}

func (db *DB) readPipelineJobs(ctx context.Context, runID string) ([]PipelineJob, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, run_id, job_key, display_name, stage, needs, matrix, node_id, status, reason, attempt, started_at, finished_at
		FROM pipeline_jobs WHERE run_id = ? ORDER BY rowid
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineJob
	for rows.Next() {
		var j PipelineJob
		var started, finished sql.NullString
		if err := rows.Scan(&j.ID, &j.RunID, &j.Key, &j.DisplayName, &j.Stage, &j.NeedsJSON, &j.MatrixJSON, &j.NodeID,
			&j.Status, &j.Reason, &j.Attempt, &started, &finished); err != nil {
			return nil, fmt.Errorf("store: scan pipeline job: %w", err)
		}
		if j.StartedAt, err = parseTimePtr(started); err != nil {
			return nil, fmt.Errorf("store: parse job started_at: %w", err)
		}
		if j.FinishedAt, err = parseTimePtr(finished); err != nil {
			return nil, fmt.Errorf("store: parse job finished_at: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (db *DB) readPipelineSteps(ctx context.Context, runID string) ([]PipelineStep, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.job_id, s.idx, s.name, s.kind, s.status, s.reason, s.exit_code, s.attempt, s.started_at, s.finished_at
		FROM pipeline_steps s JOIN pipeline_jobs j ON j.id = s.job_id
		WHERE j.run_id = ? ORDER BY s.job_id, s.idx
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline steps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineStep
	for rows.Next() {
		var s PipelineStep
		var exit sql.NullInt64
		var started, finished sql.NullString
		if err := rows.Scan(&s.JobID, &s.Index, &s.Name, &s.Kind, &s.Status, &s.Reason, &exit, &s.Attempt, &started, &finished); err != nil {
			return nil, fmt.Errorf("store: scan pipeline step: %w", err)
		}
		if exit.Valid {
			v := int(exit.Int64)
			s.ExitCode = &v
		}
		if s.StartedAt, err = parseTimePtr(started); err != nil {
			return nil, fmt.Errorf("store: parse step started_at: %w", err)
		}
		if s.FinishedAt, err = parseTimePtr(finished); err != nil {
			return nil, fmt.Errorf("store: parse step finished_at: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetPipelineJobStatus updates a job's status, reason, node, and attempt.
// started and finished are recorded when non-nil.
func (db *DB) SetPipelineJobStatus(ctx context.Context, id, status, reason, nodeID string, attempt int, started, finished *time.Time) error {
	if _, err := db.ExecContext(ctx, `
		UPDATE pipeline_jobs SET status = ?, reason = ?, node_id = CASE WHEN ? = '' THEN node_id ELSE ? END, attempt = ?,
			started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at)
		WHERE id = ?
	`, status, reason, nodeID, nodeID, attempt, formatTimePtr(started), formatTimePtr(finished), id); err != nil {
		return fmt.Errorf("store: set pipeline job %q status: %w", id, err)
	}
	return nil
}

// SetPipelineStepStatus updates one step's status, reason, exit code, and
// attempt. started and finished are recorded when non-nil.
func (db *DB) SetPipelineStepStatus(ctx context.Context, jobID string, idx int, status, reason string, exitCode *int, attempt int, started, finished *time.Time) error {
	var exit sql.NullInt64
	if exitCode != nil {
		exit = sql.NullInt64{Int64: int64(*exitCode), Valid: true}
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE pipeline_steps SET status = ?, reason = ?, exit_code = ?, attempt = ?,
			started_at = COALESCE(?, started_at), finished_at = COALESCE(?, finished_at)
		WHERE job_id = ? AND idx = ?
	`, status, reason, exit, attempt, formatTimePtr(started), formatTimePtr(finished), jobID, idx); err != nil {
		return fmt.Errorf("store: set pipeline step %q/%d status: %w", jobID, idx, err)
	}
	return nil
}

// AppendPipelineLogs inserts log lines in one transaction.
func (db *DB) AppendPipelineLogs(ctx context.Context, lines []PipelineLogLine) error {
	if len(lines) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: append pipeline logs: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, l := range lines {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO pipeline_logs (run_id, job_key, step_idx, stream, line, created_at) VALUES (?, ?, ?, ?, ?, ?)
		`, l.RunID, l.JobKey, l.StepIndex, l.Stream, l.Line, formatTime(l.CreatedAt)); err != nil {
			return fmt.Errorf("store: insert pipeline log: %w", err)
		}
	}
	return tx.Commit()
}

// CountPipelineLogs returns how many log lines a run has stored.
func (db *DB) CountPipelineLogs(ctx context.Context, runID string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pipeline_logs WHERE run_id = ?`, runID).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count pipeline logs: %w", err)
	}
	return n, nil
}

// ListPipelineLogs returns a run's log lines with ID greater than afterID.
// jobKey, when non-empty, filters to one job. limit <= 0 defaults to 1000.
func (db *DB) ListPipelineLogs(ctx context.Context, runID, jobKey string, afterID int64, limit int) ([]PipelineLogLine, error) {
	if limit <= 0 {
		limit = 1000
	}
	q := `SELECT id, run_id, job_key, step_idx, stream, line, created_at FROM pipeline_logs WHERE run_id = ? AND id > ?`
	args := []any{runID, afterID}
	if jobKey != "" {
		q += ` AND job_key = ?`
		args = append(args, jobKey)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY id LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline logs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineLogLine
	for rows.Next() {
		var l PipelineLogLine
		var created string
		if err := rows.Scan(&l.ID, &l.RunID, &l.JobKey, &l.StepIndex, &l.Stream, &l.Line, &created); err != nil {
			return nil, fmt.Errorf("store: scan pipeline log: %w", err)
		}
		if l.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse pipeline log time: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CreatePipelineApproval opens a gate; a gate that already exists for the
// job and step is left untouched so resumption is idempotent.
func (db *DB) CreatePipelineApproval(ctx context.Context, a PipelineApproval) error {
	if _, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO pipeline_approvals (run_id, job_id, step_idx, message, required_ability, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, a.RunID, a.JobID, a.StepIndex, a.Message, a.RequiredAbility, formatTimePtr(a.ExpiresAt), formatTime(a.CreatedAt)); err != nil {
		return fmt.Errorf("store: create pipeline approval: %w", err)
	}
	return nil
}

// ListPipelineApprovals returns a run's approval gates, oldest first.
func (db *DB) ListPipelineApprovals(ctx context.Context, runID string) ([]PipelineApproval, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, run_id, job_id, step_idx, message, required_ability, decision, decided_by, comment, expires_at, created_at, decided_at
		FROM pipeline_approvals WHERE run_id = ? ORDER BY id
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline approvals: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineApproval
	for rows.Next() {
		var a PipelineApproval
		var expires, decided sql.NullString
		var created string
		if err := rows.Scan(&a.ID, &a.RunID, &a.JobID, &a.StepIndex, &a.Message, &a.RequiredAbility, &a.Decision, &a.DecidedBy,
			&a.Comment, &expires, &created, &decided); err != nil {
			return nil, fmt.Errorf("store: scan pipeline approval: %w", err)
		}
		if a.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse approval created_at: %w", err)
		}
		if a.ExpiresAt, err = parseTimePtr(expires); err != nil {
			return nil, fmt.Errorf("store: parse approval expires_at: %w", err)
		}
		if a.DecidedAt, err = parseTimePtr(decided); err != nil {
			return nil, fmt.Errorf("store: parse approval decided_at: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecidePipelineApproval records a decision on a still-pending gate. It
// returns false when the gate was already decided.
func (db *DB) DecidePipelineApproval(ctx context.Context, id int64, decision, by, comment string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE pipeline_approvals SET decision = ?, decided_by = ?, comment = ?, decided_at = ? WHERE id = ? AND decision = ''
	`, decision, by, comment, formatTime(now), id)
	if err != nil {
		return false, fmt.Errorf("store: decide pipeline approval %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
