package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrOneShotJobNotFound is returned when no job has the requested id.
var ErrOneShotJobNotFound = errors.New("store: one-shot job not found")

// OneShotJob is the persisted rollup of one run-to-completion job.
type OneShotJob struct {
	ID             string
	App            string
	TaskID         string
	State          string
	Completions    int
	Parallelism    int
	BackoffLimit   int
	Succeeded      int
	FailedAttempts int
	Active         int
	RunIDs         []string
	Message        string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

const oneShotJobCols = `id, app, task_id, state, completions, parallelism, backoff_limit, succeeded, failed_attempts, active, run_ids, message, started_at, finished_at`

// SaveOneShotJob inserts or replaces the job summary.
func (db *DB) SaveOneShotJob(ctx context.Context, j OneShotJob) error {
	ids, err := json.Marshal(append([]string{}, j.RunIDs...))
	if err != nil {
		return fmt.Errorf("store: encode one-shot run ids: %w", err)
	}
	finished := ""
	if j.FinishedAt != nil {
		finished = j.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO oneshot_jobs (`+oneShotJobCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET state = excluded.state, succeeded = excluded.succeeded,
			failed_attempts = excluded.failed_attempts, active = excluded.active, run_ids = excluded.run_ids,
			message = excluded.message, finished_at = excluded.finished_at
	`, j.ID, j.App, j.TaskID, j.State, j.Completions, j.Parallelism, j.BackoffLimit, j.Succeeded, j.FailedAttempts,
		j.Active, string(ids), j.Message, j.StartedAt.UTC().Format(time.RFC3339Nano), finished)
	if err != nil {
		return fmt.Errorf("store: save one-shot job %q: %w", j.ID, err)
	}
	return nil
}

// GetOneShotJob returns the job with id, or ErrOneShotJobNotFound.
func (db *DB) GetOneShotJob(ctx context.Context, id string) (OneShotJob, error) {
	row := db.QueryRowContext(ctx, `SELECT `+oneShotJobCols+` FROM oneshot_jobs WHERE id = ?`, id)
	var (
		j                    OneShotJob
		ids, started, finish string
	)
	err := row.Scan(&j.ID, &j.App, &j.TaskID, &j.State, &j.Completions, &j.Parallelism, &j.BackoffLimit,
		&j.Succeeded, &j.FailedAttempts, &j.Active, &ids, &j.Message, &started, &finish)
	if errors.Is(err, sql.ErrNoRows) {
		return OneShotJob{}, ErrOneShotJobNotFound
	}
	if err != nil {
		return OneShotJob{}, fmt.Errorf("store: get one-shot job %q: %w", id, err)
	}
	if err := json.Unmarshal([]byte(ids), &j.RunIDs); err != nil {
		return OneShotJob{}, fmt.Errorf("store: decode one-shot run ids for %q: %w", id, err)
	}
	j.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	if finish != "" {
		if t, perr := time.Parse(time.RFC3339Nano, finish); perr == nil {
			j.FinishedAt = &t
		}
	}
	return j, nil
}

// FailInterruptedOneShotJobs marks every job still Running as Failed with
// reason. Call it once at boot, before any new job starts: a job left
// Running by a dead process is never resumed, so its command cannot run twice.
func (db *DB) FailInterruptedOneShotJobs(ctx context.Context, reason string, now time.Time) (int, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE oneshot_jobs SET state = 'Failed', active = 0, message = ?, finished_at = ?
		WHERE state = 'Running'
	`, reason, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("store: fail interrupted one-shot jobs: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PruneOneShotJobs deletes all but the newest keep finished jobs and returns
// the hidden task ids they used, so the caller can drop those rows too.
func (db *DB) PruneOneShotJobs(ctx context.Context, keep int) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, task_id FROM oneshot_jobs WHERE state != 'Running'
		ORDER BY started_at DESC LIMIT -1 OFFSET ?
	`, keep)
	if err != nil {
		return nil, fmt.Errorf("store: list one-shot jobs to prune: %w", err)
	}
	var ids, tasks []string
	for rows.Next() {
		var id, task string
		if err := rows.Scan(&id, &task); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("store: scan one-shot job: %w", err)
		}
		ids = append(ids, id)
		if task != "" {
			tasks = append(tasks, task)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("store: list one-shot jobs to prune: %w", err)
	}
	_ = rows.Close()
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `DELETE FROM oneshot_jobs WHERE id = ?`, id); err != nil {
			return nil, fmt.Errorf("store: delete one-shot job %q: %w", id, err)
		}
	}
	return tasks, nil
}
