package telemetry

import (
	"context"
	"fmt"
	"time"
)

// Span is one trace span as stored: identifiers are lowercase hex, times are
// unix nanoseconds, and attrs is the span's attributes as a JSON object.
type Span struct {
	TraceID       string
	SpanID        string
	ParentID      string
	ResourceID    string
	Name          string
	Kind          int
	StartNs       int64
	EndNs         int64
	StatusCode    int // 0 unset, 1 ok, 2 error
	StatusMessage string
	Attrs         string
}

// TraceSummary is one trace as the list shows it.
type TraceSummary struct {
	TraceID    string `json:"trace_id"`
	RootName   string `json:"root_name"`
	StartNs    int64  `json:"start_ns"`
	DurationNs int64  `json:"duration_ns"`
	Spans      int    `json:"spans"`
	Errors     int    `json:"errors"`
}

// TraceFilter narrows ListTraces.
type TraceFilter struct {
	ResourceID string
	From, To   time.Time
	MinMillis  int64
	ErrorsOnly bool
	Limit      int
}

// WriteSpans stores spans, ignoring any whose (trace, span) id is already
// there so a retried export is harmless.
func (db *DB) WriteSpans(ctx context.Context, spans []Span) error {
	if len(spans) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("telemetry: begin span write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO spans
		(trace_id, span_id, parent_id, resource_id, name, kind, start_ns, end_ns, status_code, status_message, attrs)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("telemetry: prepare span write: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, s := range spans {
		if _, err := stmt.ExecContext(ctx, s.TraceID, s.SpanID, s.ParentID, s.ResourceID, s.Name, s.Kind,
			s.StartNs, s.EndNs, s.StatusCode, s.StatusMessage, s.Attrs); err != nil {
			return fmt.Errorf("telemetry: write span: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("telemetry: commit span write: %w", err)
	}
	return nil
}

// ListTraces returns the newest matching traces first. A trace is matched by
// any of its spans being in the window, and its duration spans all of its
// stored spans.
func (db *DB) ListTraces(ctx context.Context, f TraceFilter) ([]TraceSummary, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	from, to := int64(0), time.Now().Add(time.Hour).UnixNano()
	if !f.From.IsZero() {
		from = f.From.UnixNano()
	}
	if !f.To.IsZero() {
		to = f.To.UnixNano()
	}
	rows, err := db.QueryContext(ctx, `
		SELECT trace_id,
		       COALESCE(MAX(CASE WHEN parent_id = '' THEN name END), MIN(name)),
		       MIN(start_ns), MAX(end_ns) - MIN(start_ns), COUNT(*),
		       SUM(CASE WHEN status_code = 2 THEN 1 ELSE 0 END)
		FROM spans
		WHERE trace_id IN (
			SELECT DISTINCT trace_id FROM spans WHERE resource_id = ? AND start_ns >= ? AND start_ns < ?)
		GROUP BY trace_id
		HAVING (MAX(end_ns) - MIN(start_ns)) >= ? AND (? = 0 OR SUM(CASE WHEN status_code = 2 THEN 1 ELSE 0 END) > 0)
		ORDER BY MIN(start_ns) DESC
		LIMIT ?`, f.ResourceID, from, to, f.MinMillis*int64(time.Millisecond), boolInt(f.ErrorsOnly), f.Limit)
	if err != nil {
		return nil, fmt.Errorf("telemetry: list traces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TraceSummary
	for rows.Next() {
		var t TraceSummary
		if err := rows.Scan(&t.TraceID, &t.RootName, &t.StartNs, &t.DurationNs, &t.Spans, &t.Errors); err != nil {
			return nil, fmt.Errorf("telemetry: scan trace: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GetTrace returns every stored span of a trace, earliest first, optionally
// limited to one resource.
func (db *DB) GetTrace(ctx context.Context, resourceID, traceID string) ([]Span, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT trace_id, span_id, parent_id, resource_id, name, kind, start_ns, end_ns, status_code, status_message, attrs
		FROM spans WHERE trace_id = ? AND (? = '' OR resource_id = ?) ORDER BY start_ns, span_id`, traceID, resourceID, resourceID)
	if err != nil {
		return nil, fmt.Errorf("telemetry: get trace: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Span
	for rows.Next() {
		var s Span
		if err := rows.Scan(&s.TraceID, &s.SpanID, &s.ParentID, &s.ResourceID, &s.Name, &s.Kind,
			&s.StartNs, &s.EndNs, &s.StatusCode, &s.StatusMessage, &s.Attrs); err != nil {
			return nil, fmt.Errorf("telemetry: scan span: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RetainSpans deletes spans that started before cutoff.
func (db *DB) RetainSpans(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM spans WHERE start_ns < ?`, cutoff.UnixNano())
	if err != nil {
		return 0, fmt.Errorf("telemetry: span retention sweep: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("telemetry: span retention rows: %w", err)
	}
	return n, nil
}
