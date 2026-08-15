package telemetry

import (
	"context"
	"fmt"
	"time"
)

// This file (deploy_log.go) is the deploy-attempt log store: build
// output from internal/deploylog.Recorder's batched flushes, keyed by
// deploy-attempt ID instead of log_entries' resource_id, and read back
// in full for a finished attempt's replay
// (web/src/routes/apps/$name/deploys/$deployId/logs.tsx). A deliberate
// sibling of logs.go's log_entries store, not a reuse of it: see
// migrations/0003_deploy_logs.sql's own comment for why a deploy
// attempt's one bounded, terminal event needs a different shape and a
// different retention policy than a running container's open-ended
// stream.

// DeployLogEntry is one line of a deploy attempt's build output.
type DeployLogEntry struct {
	AttemptID string
	// Stream is "stdout" or "stderr".
	Stream    string
	Timestamp time.Time
	Message   string
}

// WriteDeployLogBatch inserts every entry in one transaction, the same
// atomicity and "no upsert, a retried batch can produce duplicate rows"
// tradeoff WriteLogBatch already documents for the identical reason: a
// deploy attempt's output has no natural idempotency key either.
// internal/deploylog.Recorder is this method's only caller, and it
// already batches by count (see that package's own batchMaxLines) before
// calling this, matching WriteLogBatch's batching discipline.
func (db *DB) WriteDeployLogBatch(ctx context.Context, entries []DeployLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin deploy log write: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO deploy_logs (attempt_id, stream, ts, message)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("telemetry: prepare deploy log write: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, e.AttemptID, e.Stream, e.Timestamp.UnixNano(), e.Message); err != nil {
			return fmt.Errorf("telemetry: write deploy log entry for %s: %w", e.AttemptID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit deploy log write: %w", err)
	}
	return nil
}

// QueryDeployLog returns every persisted line for attemptID, oldest
// first: a full replay, not a windowed range query like QueryLogs, since
// a deploy attempt's log is a bounded, already-finished (by the time
// this is the code path serving it, see internal/api/deploys.go's SSE
// handler) event with no reason to page through it by time range. An
// empty (nil) result and a nil error both mean "no lines for this
// attempt," the same "absence is not an error" convention QueryLogs
// already establishes, covering both a genuinely empty log (the plain
// image-tag trigger path, which never calls WriteDeployLogBatch at all)
// and an attempt ID that was never written for any reason.
func (db *DB) QueryDeployLog(ctx context.Context, attemptID string) ([]DeployLogEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT attempt_id, stream, ts, message
		FROM deploy_logs
		WHERE attempt_id = ?
		ORDER BY ts ASC
	`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("telemetry: query deploy log for %s: %w", attemptID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []DeployLogEntry
	for rows.Next() {
		var e DeployLogEntry
		var tsNano int64
		if err := rows.Scan(&e.AttemptID, &e.Stream, &tsNano, &e.Message); err != nil {
			return nil, fmt.Errorf("telemetry: scan deploy log row: %w", err)
		}
		e.Timestamp = time.Unix(0, tsNano).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate deploy log rows: %w", err)
	}
	return out, nil
}

// RetainDeployLogs deletes every deploy log line older than cutoff,
// returning how many rows were removed. Mirrors RetainLogs' shape
// exactly, same "caller decides the cutoff, no hardcoded threshold"
// house rule; see cmd/levelrail/main.go's retention sweep wiring for the
// default this project actually runs with, chosen separately from
// RetainLogs' 15-day container-log default per this table's own
// "bounded, terminal event" reasoning (migrations/0003_deploy_logs.sql).
func (db *DB) RetainDeployLogs(ctx context.Context, cutoff time.Time) (deleted int64, err error) {
	res, err := db.ExecContext(ctx, `DELETE FROM deploy_logs WHERE ts < ?`, cutoff.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("telemetry: deploy log retention sweep: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("telemetry: deploy log retention sweep rows affected: %w", err)
	}
	return n, nil
}
