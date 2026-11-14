package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Plan job statuses. Expired is computed on read from the plan age, never stored.
const (
	PlanJobQueued  = "queued"
	PlanJobRunning = "running"
	PlanJobDone    = "done"
	PlanJobFailed  = "failed"
	PlanJobExpired = "expired"
)

// ErrPlanJobNotFound means no job has this id for the stack.
var ErrPlanJobNotFound = errors.New("store: plan job not found")

// StackPlanJob is one async cloud plan. Result is the masked plan response; CloudHashes
// names the engine plans it saved, so an approval can tell it was superseded.
type StackPlanJob struct {
	ID          string
	Stack       string
	Status      string
	Code        string
	Error       string
	Result      []byte
	CloudHashes []string
	Actor       string
	CreatedAt   time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	HeartbeatAt time.Time
}

func planJobTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func planJobTimeStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmtLockTime(t)
}

// InsertStackPlanJob stores a new queued job.
func (db *DB) InsertStackPlanJob(ctx context.Context, j StackPlanJob) error {
	_, err := db.ExecContext(ctx, `INSERT INTO stack_plan_jobs (id, stack, status, actor, created_at, heartbeat_at) VALUES (?, ?, ?, ?, ?, ?)`,
		j.ID, j.Stack, PlanJobQueued, j.Actor, fmtLockTime(j.CreatedAt), fmtLockTime(j.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: insert plan job: %w", err)
	}
	return nil
}

const planJobCols = "id, stack, status, code, error, result, cloud_hashes, actor, created_at, started_at, finished_at, heartbeat_at"

func scanPlanJob(sc interface{ Scan(...any) error }) (StackPlanJob, error) {
	var j StackPlanJob
	var hashes, created, started, finished, beat string
	if err := sc.Scan(&j.ID, &j.Stack, &j.Status, &j.Code, &j.Error, &j.Result, &hashes, &j.Actor, &created, &started, &finished, &beat); err != nil {
		return j, err
	}
	if hashes != "" {
		j.CloudHashes = strings.Split(hashes, ",")
	}
	j.CreatedAt, j.StartedAt, j.FinishedAt, j.HeartbeatAt = planJobTime(created), planJobTime(started), planJobTime(finished), planJobTime(beat)
	return j, nil
}

// GetStackPlanJob returns one job of a stack or ErrPlanJobNotFound.
func (db *DB) GetStackPlanJob(ctx context.Context, stack, id string) (StackPlanJob, error) {
	j, err := scanPlanJob(db.QueryRowContext(ctx, "SELECT "+planJobCols+" FROM stack_plan_jobs WHERE id = ? AND stack = ?", id, stack))
	if errors.Is(err, sql.ErrNoRows) {
		return StackPlanJob{}, ErrPlanJobNotFound
	}
	if err != nil {
		return StackPlanJob{}, fmt.Errorf("store: get plan job: %w", err)
	}
	return j, nil
}

// ListStackPlanJobsSince returns the jobs of a stack created at or after since, oldest first.
func (db *DB) ListStackPlanJobsSince(ctx context.Context, stack string, since time.Time) ([]StackPlanJob, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+planJobCols+" FROM stack_plan_jobs WHERE stack = ? AND created_at >= ? ORDER BY created_at", stack, fmtLockTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: list plan jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StackPlanJob
	for rows.Next() {
		j, err := scanPlanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan plan job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// StartStackPlanJob moves a queued job to running; false means it already finished.
func (db *DB) StartStackPlanJob(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, "UPDATE stack_plan_jobs SET status = ?, started_at = ?, heartbeat_at = ? WHERE id = ? AND status = ?",
		PlanJobRunning, fmtLockTime(now), fmtLockTime(now), id, PlanJobQueued)
	if err != nil {
		return false, fmt.Errorf("store: start plan job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// HeartbeatStackPlanJob marks a queued or running job as still owned by a live process.
func (db *DB) HeartbeatStackPlanJob(ctx context.Context, id string, now time.Time) error {
	_, err := db.ExecContext(ctx, "UPDATE stack_plan_jobs SET heartbeat_at = ? WHERE id = ? AND status IN (?, ?)",
		fmtLockTime(now), id, PlanJobQueued, PlanJobRunning)
	return err
}

// FinishStackPlanJob records the outcome once; false means the job had already ended.
func (db *DB) FinishStackPlanJob(ctx context.Context, j StackPlanJob) (bool, error) {
	res, err := db.ExecContext(ctx, "UPDATE stack_plan_jobs SET status = ?, code = ?, error = ?, result = ?, cloud_hashes = ?, finished_at = ? WHERE id = ? AND status IN (?, ?)",
		j.Status, j.Code, j.Error, j.Result, strings.Join(j.CloudHashes, ","), planJobTimeStr(j.FinishedAt), j.ID, PlanJobQueued, PlanJobRunning)
	if err != nil {
		return false, fmt.Errorf("store: finish plan job: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// InterruptStalePlanJobs fails queued or running jobs whose owner stopped beating, as after a restart.
func (db *DB) InterruptStalePlanJobs(ctx context.Context, code, msg string, staleBefore, now time.Time) error {
	_, err := db.ExecContext(ctx, "UPDATE stack_plan_jobs SET status = ?, code = ?, error = ?, finished_at = ? WHERE status IN (?, ?) AND heartbeat_at < ?",
		PlanJobFailed, code, msg, fmtLockTime(now), PlanJobQueued, PlanJobRunning, fmtLockTime(staleBefore))
	if err != nil {
		return fmt.Errorf("store: interrupt plan jobs: %w", err)
	}
	return nil
}

// PruneStackPlanJobs deletes finished jobs created before the retention cutoff.
func (db *DB) PruneStackPlanJobs(ctx context.Context, before time.Time) error {
	_, err := db.ExecContext(ctx, "DELETE FROM stack_plan_jobs WHERE created_at < ? AND status IN (?, ?)", fmtLockTime(before), PlanJobDone, PlanJobFailed)
	if err != nil {
		return fmt.Errorf("store: prune plan jobs: %w", err)
	}
	return nil
}
