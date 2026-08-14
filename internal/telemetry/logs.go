package telemetry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// This file (logs.go) is the node-local log store: read a container's
// log stream directly (per TASKS.md 2.2) from the Docker Engine API
// (never the json-file driver's on-disk files), chunk it into batches,
// index it for full-text search, and flag
// lines that parse as JSON so a future log viewer (TASKS.md 2.4) can
// render them distinctly. Lives in the same package and the same
// telemetry.db as store.go's metric samples: ADR 008 frames metrics and
// logs as the same "node-local telemetry" category, and ADR 009's
// write-isolation reasoning was applied against internal/store
// specifically (a desired-state database with an entirely different
// write pattern), not against telemetry's own two data domains, which
// share both a write cadence (bursty, per-batch writes) and a retention
// model (a periodic delete-older-than sweep).
//
// Compression (the log store's "chunk, compress, index" design goal):
// deliberately not an explicit step here. FTS5 (see
// migrations/0002_log_entries.sql) needs plaintext access to message to
// build and query its token index; compressing message at the row level
// would mean decompressing before every search, which defeats the point
// of having an index at all, or storing both a compressed and an
// uncompressed copy, which is worse than not compressing. The goal
// "compress" is meant to solve (don't let a log firehose bloat storage
// unchecked) is instead solved by: batched writes (fewer, larger
// transactions, see logBatchMaxLines/
// logBatchMaxWait below) rather than one write per line, FTS5's
// external-content mode (log_entries_fts stores token postings only, not
// a second copy of every message), and the same bounded-retention sweep
// ADR 009 already applies to metric_samples. This mirrors ADR 009's own
// reasoning template: purpose-built and sized to this project's actual
// scale, not a general-purpose system's feature set.
const (
	// logBatchMaxLines is how many buffered lines trigger an immediate
	// flush to SQLite, regardless of how long they took to accumulate.
	// Docker's log stream is a firehose (potentially many lines per
	// second per container); writing one SQLite transaction per line
	// would make transaction overhead dominate over the actual write, the
	// same reasoning WriteSamples' single-transaction-per-tick already
	// applies to metric collection, adapted here to a line count instead
	// of a fixed collection tick since log lines arrive at an unpredictable
	// rate rather than every 15 seconds.
	logBatchMaxLines = 200
	// logBatchMaxWait bounds how stale a live "tail" view can be when a
	// container is producing output slower than logBatchMaxLines: a batch
	// flushes on whichever of the two limits is hit first, so a quiet
	// container's occasional line still lands within a couple of seconds
	// instead of waiting indefinitely for 200 lines that may never come.
	logBatchMaxWait = 2 * time.Second
)

// LogEntry is one line of container log output, landed in or read back
// from the store.
type LogEntry struct {
	ResourceID string
	// Stream is "stdout" or "stderr".
	Stream    string
	Timestamp time.Time
	Message   string
	// Structured is true when Message's full text parsed as a JSON
	// object; see classifyLine.
	Structured bool
	// FieldsJSON holds Message's parsed JSON when Structured is true,
	// stored in its own column (per TASKS.md 2.2's "store the parsed JSON
	// separately from the raw line") so a future log viewer doesn't need
	// to re-parse Message to tell structured and plain lines apart or to
	// render one differently from the other. Empty when Structured is
	// false.
	FieldsJSON string
}

// WriteLogBatch inserts every entry in one transaction, the same
// atomicity WriteSamples already gives metric collection ticks, applied
// here to one log batch (see logBatchMaxLines/logBatchMaxWait). Unlike
// WriteSamples, there's no ON CONFLICT upsert: metric samples have a
// natural idempotency key (resource, metric, timestamp) a retried
// collection tick can safely overwrite, but log lines don't, since a
// container legitimately emitting the same text twice (e.g. two "OK"
// lines) is normal, not a duplicate to be coalesced. This is a real,
// documented gap: if a batch write fails partway and the caller decides
// to retry with the same lines, retried lines land as new rows rather
// than replacing anything, so a transient write failure during
// streaming can produce duplicate log rows in rare cases. Closing that
// gap would need its own idempotency key (e.g. a source-assigned
// sequence number per container), deliberately deferred rather than
// solved silently here.
func (db *DB) WriteLogBatch(ctx context.Context, entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin log write: %w", err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO log_entries (resource_id, stream, ts, message, structured, fields_json)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("telemetry: prepare log write: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, e := range entries {
		structured := 0
		var fieldsJSON sql.NullString
		if e.Structured {
			structured = 1
			fieldsJSON = sql.NullString{String: e.FieldsJSON, Valid: true}
		}
		if _, err := stmt.ExecContext(ctx, e.ResourceID, e.Stream, e.Timestamp.UnixNano(), e.Message, structured, fieldsJSON); err != nil {
			return fmt.Errorf("telemetry: write log entry for %s: %w", e.ResourceID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit log write: %w", err)
	}
	return nil
}

// QueryLogs returns every log entry for resourceID with a timestamp in
// [from, to], oldest first, optionally filtered to lines whose message
// matches query. An empty query returns every line in range with no text
// filter applied. A non-empty query runs against log_entries_fts (see
// migrations/0002_log_entries.sql), wrapped as a single quoted phrase
// (double quotes inside query are escaped by doubling, FTS5's own escape
// convention) rather than passed through as raw FTS5 query syntax: this
// keeps the query API a plain substring-ish search from a caller's point
// of view, not something that can fail with an FTS5 syntax error just
// because the search text happened to contain a character FTS5's query
// grammar treats specially (e.g. a bare "-" or unbalanced quote).
//
// An empty (nil) result and a nil error both mean "no lines matched,"
// not an error, the same convention Query already establishes for metric
// samples.
func (db *DB) QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]LogEntry, error) {
	var rows *sql.Rows
	var err error

	if query == "" {
		rows, err = db.QueryContext(ctx, `
			SELECT resource_id, stream, ts, message, structured, fields_json
			FROM log_entries
			WHERE resource_id = ? AND ts BETWEEN ? AND ?
			ORDER BY ts ASC
		`, resourceID, from.UnixNano(), to.UnixNano())
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT l.resource_id, l.stream, l.ts, l.message, l.structured, l.fields_json
			FROM log_entries l
			JOIN log_entries_fts f ON f.rowid = l.id
			WHERE l.resource_id = ? AND l.ts BETWEEN ? AND ? AND log_entries_fts MATCH ?
			ORDER BY l.ts ASC
		`, resourceID, from.UnixNano(), to.UnixNano(), ftsPhrase(query))
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: query logs for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []LogEntry
	for rows.Next() {
		var (
			e          LogEntry
			tsNano     int64
			structured int
			fieldsJSON sql.NullString
		)
		if err := rows.Scan(&e.ResourceID, &e.Stream, &tsNano, &e.Message, &structured, &fieldsJSON); err != nil {
			return nil, fmt.Errorf("telemetry: scan log row: %w", err)
		}
		e.Timestamp = time.Unix(0, tsNano).UTC()
		e.Structured = structured != 0
		e.FieldsJSON = fieldsJSON.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate log rows: %w", err)
	}
	return out, nil
}

// ftsPhrase wraps query as a single FTS5 phrase match, escaping any
// double quote in query by doubling it, FTS5's own in-phrase escape
// convention for a literal '"'.
func ftsPhrase(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

// RetainLogs deletes every log entry older than cutoff, returning how
// many rows were removed. Mirrors Retain's shape exactly (same signature
// pattern, same "caller decides the cutoff, no hardcoded threshold"
// house rule); the FTS index stays in sync
// automatically via log_entries_ad (migrations/0002_log_entries.sql),
// not because of anything this method does itself.
func (db *DB) RetainLogs(ctx context.Context, cutoff time.Time) (deleted int64, err error) {
	res, err := db.ExecContext(ctx, `DELETE FROM log_entries WHERE ts < ?`, cutoff.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("telemetry: log retention sweep: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("telemetry: log retention sweep rows affected: %w", err)
	}
	return n, nil
}

// classifyLine decides whether raw is a structured (JSON) log line.
// Only a JSON object counts: a bare JSON string, number, or array is
// technically valid JSON per encoding/json, but not what "the app emits
// JSON" (TASKS.md 2.2) actually means in practice, every structured
// logging library (zap, logrus, pino, zerolog, ...) emits one JSON
// object per line, never a bare scalar or array. Restricting to objects
// avoids misclassifying an ordinary plain-text line that happens to be a
// valid JSON number (e.g. a line that's just "42") as structured.
func classifyLine(raw string) (structured bool, fieldsJSON string) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return false, ""
	}
	if !json.Valid([]byte(trimmed)) {
		return false, ""
	}
	return true, trimmed
}

// LogTarget is one container a LogCollector should stream logs from.
type LogTarget struct {
	ResourceID  string
	ContainerID string
}

// LogSource is the narrow Docker surface a LogCollector needs: nothing
// about container discovery, only "give me a demultiplexed log stream
// for this ID." *docker.Client satisfies this structurally. Deliberately
// not part of docker.Runtime, the same reasoning StatsSource's doc
// comment already gives for the same choice: adding this to Runtime
// would mean updating every existing fake Runtime implementation across
// the codebase for a capability only this package needs.
type LogSource interface {
	Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan docker.LogLine, <-chan error)
}

// LogCollector streams logs from a caller-supplied set of targets and
// writes what it receives to a Store, batching writes per
// logBatchMaxLines/logBatchMaxWait.
type LogCollector struct {
	source LogSource
	store  *DB
	logger *slog.Logger
}

// NewLogCollector builds a LogCollector. logger defaults to
// slog.Default() if nil.
func NewLogCollector(source LogSource, store *DB, logger *slog.Logger) *LogCollector {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogCollector{source: source, store: store, logger: logger}
}

// StreamOne follows target's log stream (Follow: true, from the current
// point forward) until ctx is cancelled or the stream ends on its own
// (the container stopped), batching lines per logBatchMaxLines/
// logBatchMaxWait and writing each batch to the store. Blocking: callers
// run it in its own goroutine per target, one long-lived subscription
// per running container, not a polling tick like Collector.CollectOnce:
// logs are a push stream from Docker, not a point-in-time snapshot, so
// this package's two collectors have genuinely different shapes even
// though they share a store and a retention pattern.
func (lc *LogCollector) StreamOne(ctx context.Context, target LogTarget) error {
	lines, errs := lc.source.Logs(ctx, target.ContainerID, true, time.Time{})

	ticker := time.NewTicker(logBatchMaxWait)
	defer ticker.Stop()

	batch := make([]LogEntry, 0, logBatchMaxLines)
	flush := func(writeCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if err := lc.store.WriteLogBatch(writeCtx, batch); err != nil {
			lc.logger.Error("telemetry: write log batch failed",
				slog.String("resource_id", target.ResourceID), slog.String("error", err.Error()))
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			// ctx is already cancelled here, so the shutdown flush uses a
			// short-lived detached context instead of ctx itself:
			// WriteLogBatch's BeginTx/ExecContext calls would otherwise
			// fail immediately against an already-Done context, silently
			// dropping whatever was buffered right when a clean shutdown
			// is exactly when it matters most not to lose it.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			flush(flushCtx)
			cancel()
			return ctx.Err()
		case err, ok := <-errs:
			if !ok {
				errs = nil // closed: disable this case so a closed error channel can't busy-loop the select
				continue
			}
			if err != nil {
				lc.logger.Error("telemetry: log stream error",
					slog.String("resource_id", target.ResourceID), slog.String("error", err.Error()))
			}
		case line, ok := <-lines:
			if !ok {
				flush(ctx)
				return nil // the stream ended on its own, e.g. the container stopped
			}
			batch = append(batch, toLogEntry(target.ResourceID, line))
			if len(batch) >= logBatchMaxLines {
				flush(ctx)
			}
		case <-ticker.C:
			flush(ctx)
		}
	}
}

// toLogEntry converts one demultiplexed docker.LogLine into the LogEntry
// shape the store persists, classifying it as structured or plain and
// falling back to the local clock for Timestamp when Docker's own
// timestamp didn't parse (docker.LogLine.Timestamp's own doc comment
// covers when that happens).
func toLogEntry(resourceID string, line docker.LogLine) LogEntry {
	ts := line.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	structured, fieldsJSON := classifyLine(line.Message)
	return LogEntry{
		ResourceID: resourceID,
		Stream:     line.Stream,
		Timestamp:  ts,
		Message:    line.Message,
		Structured: structured,
		FieldsJSON: fieldsJSON,
	}
}

// Run derives targets via targetsFunc on a resync interval and keeps
// exactly one StreamOne goroutine alive per currently-desired container
// ID, starting new ones as containers appear and cancelling ones for
// containers no longer desired, until ctx is done. This is the
// streaming-collector equivalent of Collector.Run's "re-derive targets
// every tick" principle: the set of things being collected stays current
// as services are created, redeployed, or removed, without needing a
// separate mechanism to notice that a target disappeared mid-stream
// versus never existed.
func (lc *LogCollector) Run(ctx context.Context, resync time.Duration, targetsFunc func(context.Context) ([]LogTarget, error)) error {
	active := make(map[string]context.CancelFunc) // keyed by ContainerID

	reconcile := func() {
		targets, err := targetsFunc(ctx)
		if err != nil {
			lc.logger.Error("telemetry: list log targets failed", slog.String("error", err.Error()))
			return
		}

		wanted := make(map[string]LogTarget, len(targets))
		for _, t := range targets {
			wanted[t.ContainerID] = t
		}

		for id, cancel := range active {
			if _, ok := wanted[id]; !ok {
				cancel()
				delete(active, id)
			}
		}

		for id, target := range wanted {
			if _, ok := active[id]; ok {
				continue
			}
			streamCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is stored in active and called from the two loops above (target removed, or Run shutting down), gosec's static check can't trace a func value stored in a map
			active[id] = cancel
			go func(target LogTarget) {
				if err := lc.StreamOne(streamCtx, target); err != nil && !errors.Is(err, context.Canceled) {
					lc.logger.Warn("telemetry: log stream ended", slog.String("resource_id", target.ResourceID), slog.String("error", err.Error()))
				}
			}(target)
		}
	}

	reconcile() // start immediately, don't wait a full resync interval

	ticker := time.NewTicker(resync)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			for _, cancel := range active {
				cancel()
			}
			return ctx.Err()
		case <-ticker.C:
			reconcile()
		}
	}
}
