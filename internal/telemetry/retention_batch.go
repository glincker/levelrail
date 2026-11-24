package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"
)

// Fallbacks used when no settings source is wired (tests, early boot).
const (
	defaultRetentionBatchRows = 5000
	defaultRetentionBudget    = 10 * time.Second
	defaultRetentionPause     = 50 * time.Millisecond
)

// RetentionTuning bounds how hard one retention sweep may hit the single telemetry connection.
type RetentionTuning struct {
	BatchRows   int
	SweepBudget time.Duration
	Pause       time.Duration
}

var retentionTuningSource atomic.Pointer[func() RetentionTuning]

// SetRetentionTuningSource makes sweeps follow fn, so a saved setting applies to the next batch.
func SetRetentionTuningSource(fn func() RetentionTuning) { retentionTuningSource.Store(&fn) }

func currentRetentionTuning() RetentionTuning {
	t := RetentionTuning{BatchRows: defaultRetentionBatchRows, SweepBudget: defaultRetentionBudget, Pause: defaultRetentionPause}
	if fn := retentionTuningSource.Load(); fn != nil {
		t = (*fn)()
	}
	if t.BatchRows < 1 {
		t.BatchRows = defaultRetentionBatchRows
	}
	if t.SweepBudget <= 0 {
		t.SweepBudget = defaultRetentionBudget
	}
	t.Pause = max(t.Pause, 0)
	return t
}

// retentionTarget names one table's boundary lookup (the key of the Nth oldest expired row)
// and its delete up to that key. Both take the cutoff so a row newer than it is never removed.
type retentionTarget struct {
	boundary string // args: cutoff, offset; returns the key
	del      string // args: key, cutoff
}

var (
	logRetention = retentionTarget{
		boundary: `SELECT id FROM log_entries WHERE ts < ? ORDER BY id LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM log_entries WHERE id <= ? AND ts < ?`,
	}
	rawSampleRetention = retentionTarget{
		boundary: `SELECT ts FROM metric_samples WHERE ts < ? ORDER BY ts LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM metric_samples WHERE ts <= ? AND ts < ?`,
	}
	minuteRollupRetention = retentionTarget{
		boundary: `SELECT ts FROM metric_rollups WHERE tier = 60 AND ts < ? ORDER BY ts LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM metric_rollups WHERE tier = 60 AND ts <= ? AND ts < ?`,
	}
	hourRollupRetention = retentionTarget{
		boundary: `SELECT ts FROM metric_rollups WHERE tier = 3600 AND ts < ? ORDER BY ts LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM metric_rollups WHERE tier = 3600 AND ts <= ? AND ts < ?`,
	}
	breakdownRetention = retentionTarget{
		boundary: `SELECT ts FROM request_breakdown WHERE ts < ? ORDER BY ts LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM request_breakdown WHERE ts <= ? AND ts < ?`,
	}
	upstreamRetention = retentionTarget{
		boundary: `SELECT ts FROM request_upstreams WHERE ts < ? ORDER BY ts LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM request_upstreams WHERE ts <= ? AND ts < ?`,
	}
	audienceRetention = retentionTarget{
		boundary: `SELECT ts FROM request_audience WHERE ts < ? ORDER BY ts LIMIT 1 OFFSET ?`,
		del:      `DELETE FROM request_audience WHERE ts <= ? AND ts < ?`,
	}
)

// sweepBatched deletes rows older than cutoff one bounded batch at a time, pausing between
// batches so readers get the connection. It stops quietly when deadline passes; the next sweep resumes.
func (db *DB) sweepBatched(ctx context.Context, t retentionTarget, cutoff int64, deadline time.Time) (int64, error) {
	tune := currentRetentionTuning()
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, fmt.Errorf("telemetry: retention sweep: %w", err)
		}
		key := int64(math.MaxInt64)
		last := false
		err := db.QueryRowContext(ctx, t.boundary, cutoff, tune.BatchRows-1).Scan(&key)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			last = true
		case err != nil:
			return total, fmt.Errorf("telemetry: retention boundary: %w", err)
		}
		res, err := db.ExecContext(ctx, t.del, key, cutoff)
		if err != nil {
			return total, fmt.Errorf("telemetry: retention delete: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("telemetry: retention rows affected: %w", err)
		}
		total += n
		if last || !time.Now().Before(deadline) {
			return total, nil
		}
		select {
		case <-ctx.Done():
			return total, fmt.Errorf("telemetry: retention sweep: %w", ctx.Err())
		case <-time.After(tune.Pause):
		}
	}
}
