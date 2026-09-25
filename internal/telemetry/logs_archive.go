package telemetry

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// LogStreamResult describes how far StreamLogs got.
type LogStreamResult struct {
	Lines int64
	// EndNs is the exclusive upper bound actually covered; it is below the
	// requested bound only when the line cap truncated the range.
	EndNs int64
}

// DistinctLogResources lists resources with at least one log line in [fromNs, toNs).
func (db *DB) DistinctLogResources(ctx context.Context, fromNs, toNs int64) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT resource_id FROM log_entries WHERE ts >= ? AND ts < ? ORDER BY resource_id`, fromNs, toNs)
	if err != nil {
		return nil, fmt.Errorf("telemetry: list log resources: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("telemetry: scan log resource: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("telemetry: iterate log resources: %w", err)
	}
	return out, nil
}

// StreamLogs calls fn for each line of resourceID in [fromNs, toNs) in
// timestamp order, at most maxLines. When the cap is hit the range ends at
// a timestamp boundary so a follow-up call starting at EndNs neither
// repeats nor skips lines.
func (db *DB) StreamLogs(ctx context.Context, resourceID string, fromNs, toNs int64, maxLines int, fn func(LogEntry) error) (LogStreamResult, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT resource_id, stream, ts, message, structured, fields_json
		FROM log_entries WHERE resource_id = ? AND ts >= ? AND ts < ?
		ORDER BY ts ASC, id ASC LIMIT ?
	`, resourceID, fromNs, toNs, maxLines+1)
	if err != nil {
		return LogStreamResult{}, fmt.Errorf("telemetry: stream logs for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()

	var (
		res     = LogStreamResult{EndNs: toNs}
		group   []LogEntry
		seen    int
		flushed bool
	)
	flush := func() error {
		for _, e := range group {
			if err := fn(e); err != nil {
				return err
			}
			res.Lines++
		}
		flushed = flushed || len(group) > 0
		group = group[:0]
		return nil
	}
	var lastTs int64
	for rows.Next() {
		e, tsNano, err := scanStreamRow(rows)
		if err != nil {
			return LogStreamResult{}, err
		}
		seen++
		if len(group) > 0 && tsNano != lastTs {
			if err := flush(); err != nil {
				return LogStreamResult{}, err
			}
		}
		group = append(group, e)
		lastTs = tsNano
	}
	if err := rows.Err(); err != nil {
		return LogStreamResult{}, fmt.Errorf("telemetry: iterate stream logs: %w", err)
	}

	if seen > maxLines {
		if !flushed {
			res.EndNs = lastTs + 1
			return res, flush()
		}
		res.EndNs = lastTs
		return res, nil
	}
	return res, flush()
}

func scanStreamRow(rows *sql.Rows) (LogEntry, int64, error) {
	var (
		e          LogEntry
		tsNano     int64
		structured int
		fieldsJSON sql.NullString
	)
	if err := rows.Scan(&e.ResourceID, &e.Stream, &tsNano, &e.Message, &structured, &fieldsJSON); err != nil {
		return LogEntry{}, 0, fmt.Errorf("telemetry: scan stream log row: %w", err)
	}
	e.Timestamp = time.Unix(0, tsNano).UTC()
	e.Structured = structured != 0
	e.FieldsJSON = fieldsJSON.String
	return e, tsNano, nil
}
