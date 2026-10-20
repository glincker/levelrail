package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// LimitedLogsSource reads at most limit of the newest lines in a window.
type LimitedLogsSource interface {
	QueryLogsLimited(ctx context.Context, resourceID string, from, to time.Time, query string, limit int) (entries []LogEntry, truncated bool, err error)
}

// QueryLogsLimited returns the newest limit lines for resourceID in
// [from, to], oldest first, and whether older matching lines were left out.
// It reads through idx_log_entries_resource_ts and stops at limit+1 rows, so
// a rule or aggregate over a busy app never scans the whole window.
func (db *DB) QueryLogsLimited(ctx context.Context, resourceID string, from, to time.Time, query string, limit int) ([]LogEntry, bool, error) {
	if limit <= 0 {
		return nil, false, fmt.Errorf("telemetry: query logs limited for %s: limit must be positive", resourceID)
	}
	var rows *sql.Rows
	match, err := compileLogSearch(query)
	if err != nil {
		return nil, false, err
	}
	if match == "" {
		rows, err = db.QueryContext(ctx, `
			SELECT resource_id, container_id, stream, ts, message, structured, fields_json
			FROM log_entries
			WHERE resource_id = ? AND ts BETWEEN ? AND ?
			ORDER BY ts DESC
			LIMIT ?
		`, resourceID, from.UnixNano(), to.UnixNano(), limit+1)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT l.resource_id, l.container_id, l.stream, l.ts, l.message, l.structured, l.fields_json
			FROM log_entries l
			JOIN log_entries_fts f ON f.rowid = l.id
			WHERE l.resource_id = ? AND l.ts BETWEEN ? AND ? AND log_entries_fts MATCH ?
			ORDER BY l.ts DESC
			LIMIT ?
		`, resourceID, from.UnixNano(), to.UnixNano(), match, limit+1)
	}
	if err != nil {
		return nil, false, fmt.Errorf("telemetry: query logs limited for %s: %w", resourceID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]LogEntry, 0, min(limit+1, 1024))
	for rows.Next() {
		var (
			e          LogEntry
			tsNano     int64
			structured int
			fieldsJSON sql.NullString
		)
		if err := rows.Scan(&e.ResourceID, &e.ContainerID, &e.Stream, &tsNano, &e.Message, &structured, &fieldsJSON); err != nil {
			return nil, false, fmt.Errorf("telemetry: scan log row: %w", err)
		}
		e.Timestamp = time.Unix(0, tsNano).UTC()
		e.Structured = structured != 0
		e.FieldsJSON = fieldsJSON.String
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("telemetry: iterate log rows: %w", err)
	}
	truncated := len(out) > limit
	if truncated {
		out = out[:limit]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, truncated, nil
}

// QueryLogsLimited is the federated bounded read: each source that supports
// it is capped at limit, the rest fall back to QueryLogs and are trimmed.
func (f *Federator) QueryLogsLimited(ctx context.Context, resourceID string, from, to time.Time, query string, limit int) ([]LogEntry, bool, error) {
	var (
		all       []LogEntry
		truncated bool
		errs      []error
	)
	for _, src := range f.logs {
		var (
			part []LogEntry
			trim bool
			err  error
		)
		if ls, ok := src.(LimitedLogsSource); ok {
			part, trim, err = ls.QueryLogsLimited(ctx, resourceID, from, to, query, limit)
		} else {
			part, err = src.QueryLogs(ctx, resourceID, from, to, query)
			if len(part) > limit {
				part, trim = part[len(part)-limit:], true
			}
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		truncated = truncated || trim
		all = append(all, part...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Timestamp.Before(all[j].Timestamp) })
	if len(all) > limit {
		all, truncated = all[len(all)-limit:], true
	}
	return all, truncated, errors.Join(errs...)
}

// BoundedLogQuerier is the unbounded read every log source has.
type BoundedLogQuerier interface {
	QueryLogs(ctx context.Context, resourceID string, from, to time.Time, query string) ([]LogEntry, error)
}

// QueryLogsBounded reads at most limit of the newest lines from src, using the
// source's bounded read when it has one and trimming otherwise.
func QueryLogsBounded(ctx context.Context, src BoundedLogQuerier, resourceID string, from, to time.Time, query string, limit int) ([]LogEntry, bool, error) {
	if ls, ok := src.(LimitedLogsSource); ok {
		return ls.QueryLogsLimited(ctx, resourceID, from, to, query, limit)
	}
	entries, err := src.QueryLogs(ctx, resourceID, from, to, query)
	if len(entries) > limit {
		return entries[len(entries)-limit:], true, err
	}
	return entries, false, err
}
