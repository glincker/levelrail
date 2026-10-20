package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// trimBatchRows bounds one delete batch; trimMaxBatches bounds one
	// app's work per sweep so a huge backlog drains over several sweeps.
	trimBatchRows  = 1000
	trimMaxBatches = 20
	usageWindow    = 24 * time.Hour

	// logRowBytes is what one log row is charged: message plus structured fields.
	logRowBytes = `length(message) + COALESCE(length(fields_json), 0)`
)

// DefaultLogBudgetBytes reads APP_LOG_BUDGET_MB; 0 or unset means unlimited.
func DefaultLogBudgetBytes() int64 {
	mb, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("APP_LOG_BUDGET_MB")), 10, 64)
	if err != nil || mb <= 0 {
		return 0
	}
	return mb << 20
}

// LogUsage is one resource's stored log volume against its budget.
type LogUsage struct {
	ResourceID   string
	Bytes        int64
	Lines        int64
	RatePerMin   float64 // bytes per minute averaged over the last 24h
	BudgetBytes  int64   // 0 means unlimited
	BudgetCustom bool    // true when a per-app override is stored
	TrimmedLines int64
	TrimmedBytes int64
}

// ForecastDays estimates days until used reaches budget at ratePerMin; nil when
// unlimited or idle, zero when already at or over budget.
func ForecastDays(used, budget int64, ratePerMin float64) *float64 {
	if budget <= 0 || ratePerMin <= 0 {
		return nil
	}
	remaining := budget - used
	var d float64
	if remaining > 0 {
		d = float64(remaining) / (ratePerMin * 60 * 24)
	}
	return &d
}

// LogBudgetTrim reports what one enforcement pass removed for one resource.
type LogBudgetTrim struct {
	ResourceID string
	Lines      int64
	Bytes      int64
}

// GetLogBudget returns the stored override in bytes, 0 when none.
func (db *DB) GetLogBudget(ctx context.Context, resourceID string) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, `SELECT max_bytes FROM log_budgets WHERE resource_id = ?`, resourceID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("telemetry: get log budget: %w", err)
	}
	return n, nil
}

// SetLogBudget stores the override; maxBytes <= 0 clears it.
func (db *DB) SetLogBudget(ctx context.Context, resourceID string, maxBytes int64) error {
	if maxBytes <= 0 {
		return db.DeleteLogBudget(ctx, resourceID)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO log_budgets (resource_id, max_bytes, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(resource_id) DO UPDATE SET max_bytes = excluded.max_bytes, updated_at = excluded.updated_at`,
		resourceID, maxBytes, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("telemetry: set log budget: %w", err)
	}
	return nil
}

// DeleteLogBudget removes the override so the global default applies again.
func (db *DB) DeleteLogBudget(ctx context.Context, resourceID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM log_budgets WHERE resource_id = ?`, resourceID); err != nil {
		return fmt.Errorf("telemetry: delete log budget: %w", err)
	}
	return nil
}

// LogUsageAll returns usage for every resource that has log rows, an
// override, or trim history, noisiest (most stored bytes) first. It runs two
// grouped aggregates (all rows, and the last 24h) plus two tiny table reads.
func (db *DB) LogUsageAll(ctx context.Context, now time.Time, defaultMax int64) ([]LogUsage, error) {
	byID := map[string]*LogUsage{}
	get := func(id string) *LogUsage {
		u, ok := byID[id]
		if !ok {
			u = &LogUsage{ResourceID: id, BudgetBytes: defaultMax}
			byID[id] = u
		}
		return u
	}
	rows, err := db.QueryContext(ctx, `SELECT resource_id, COUNT(*), COALESCE(SUM(`+logRowBytes+`), 0) FROM log_entries GROUP BY resource_id`)
	if err != nil {
		return nil, fmt.Errorf("telemetry: log usage totals: %w", err)
	}
	for rows.Next() {
		var id string
		var lines, bytes int64
		if err := rows.Scan(&id, &lines, &bytes); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("telemetry: scan log usage: %w", err)
		}
		u := get(id)
		u.Lines, u.Bytes = lines, bytes
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	since := now.Add(-usageWindow).UnixNano()
	rows, err = db.QueryContext(ctx, `SELECT resource_id, COALESCE(SUM(`+logRowBytes+`), 0) FROM log_entries WHERE ts >= ? GROUP BY resource_id`, since)
	if err != nil {
		return nil, fmt.Errorf("telemetry: log usage rate: %w", err)
	}
	for rows.Next() {
		var id string
		var bytes int64
		if err := rows.Scan(&id, &bytes); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("telemetry: scan log rate: %w", err)
		}
		get(id).RatePerMin = float64(bytes) / usageWindow.Minutes()
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `SELECT resource_id, max_bytes FROM log_budgets`)
	if err != nil {
		return nil, fmt.Errorf("telemetry: log budgets: %w", err)
	}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("telemetry: scan log budget: %w", err)
		}
		u := get(id)
		u.BudgetBytes, u.BudgetCustom = n, true
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	rows, err = db.QueryContext(ctx, `SELECT resource_id, lines, bytes FROM log_budget_drops`)
	if err != nil {
		return nil, fmt.Errorf("telemetry: log budget drops: %w", err)
	}
	for rows.Next() {
		var id string
		var l, b int64
		if err := rows.Scan(&id, &l, &b); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("telemetry: scan log drops: %w", err)
		}
		u := get(id)
		u.TrimmedLines, u.TrimmedBytes = l, b
	}
	if err := closeRows(rows); err != nil {
		return nil, err
	}
	out := make([]LogUsage, 0, len(byID))
	for _, u := range byID {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].ResourceID < out[j].ResourceID
	})
	return out, nil
}

func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("telemetry: iterate rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("telemetry: close rows: %w", err)
	}
	return nil
}

// EnforceLogBudgets trims the oldest rows of every resource over its budget
// (override, else defaultMax). Only that resource's rows are touched, and
// each resource drains at most trimMaxBatches batches per call.
func (db *DB) EnforceLogBudgets(ctx context.Context, defaultMax int64) ([]LogBudgetTrim, error) {
	usage, err := db.LogUsageAll(ctx, time.Now(), defaultMax)
	if err != nil {
		return nil, err
	}
	var trims []LogBudgetTrim
	for _, u := range usage {
		if u.BudgetBytes <= 0 || u.Bytes <= u.BudgetBytes {
			continue
		}
		t, err := db.trimResource(ctx, u.ResourceID, u.Bytes-u.BudgetBytes)
		if t.Lines > 0 {
			trims = append(trims, t)
		}
		if err != nil {
			return trims, err
		}
	}
	return trims, nil
}

// TrimLogBudget trims one resource's oldest rows down to its budget (override,
// else defaultMax) now. The bool reports a backlog larger than one pass drains.
func (db *DB) TrimLogBudget(ctx context.Context, resourceID string, defaultMax int64) (LogBudgetTrim, bool, error) {
	budget, err := db.GetLogBudget(ctx, resourceID)
	if err != nil {
		return LogBudgetTrim{}, false, err
	}
	if budget <= 0 {
		budget = defaultMax
	}
	if budget <= 0 {
		return LogBudgetTrim{ResourceID: resourceID}, false, nil
	}
	var used int64
	err = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(`+logRowBytes+`), 0) FROM log_entries WHERE resource_id = ?`, resourceID).Scan(&used)
	if err != nil {
		return LogBudgetTrim{}, false, fmt.Errorf("telemetry: log usage for trim: %w", err)
	}
	if used <= budget {
		return LogBudgetTrim{ResourceID: resourceID}, false, nil
	}
	t, err := db.trimResource(ctx, resourceID, used-budget)
	return t, used-t.Bytes > budget, err
}

func (db *DB) trimResource(ctx context.Context, id string, excess int64) (LogBudgetTrim, error) {
	t := LogBudgetTrim{ResourceID: id}
	for batch := 0; batch < trimMaxBatches && t.Bytes < excess; batch++ {
		rows, err := db.QueryContext(ctx, `SELECT id, `+logRowBytes+` FROM log_entries WHERE resource_id = ? ORDER BY ts, id LIMIT ?`, id, trimBatchRows)
		if err != nil {
			return t, fmt.Errorf("telemetry: log budget select: %w", err)
		}
		var ids []any
		var batchBytes int64
		for rows.Next() && t.Bytes+batchBytes < excess {
			var rid, n int64
			if err := rows.Scan(&rid, &n); err != nil {
				_ = rows.Close()
				return t, fmt.Errorf("telemetry: scan log budget row: %w", err)
			}
			ids = append(ids, rid)
			batchBytes += n
		}
		if err := closeRows(rows); err != nil {
			return t, err
		}
		if len(ids) == 0 {
			break
		}
		q := `DELETE FROM log_entries WHERE resource_id = ? AND id IN (?` + strings.Repeat(",?", len(ids)-1) + `)`
		res, err := db.ExecContext(ctx, q, append([]any{id}, ids...)...)
		if err != nil {
			return t, fmt.Errorf("telemetry: log budget delete: %w", err)
		}
		n, _ := res.RowsAffected()
		t.Lines += n
		t.Bytes += batchBytes
	}
	if t.Lines > 0 {
		_, err := db.ExecContext(ctx, `INSERT INTO log_budget_drops (resource_id, lines, bytes, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(resource_id) DO UPDATE SET lines = lines + excluded.lines, bytes = bytes + excluded.bytes, updated_at = excluded.updated_at`,
			id, t.Lines, t.Bytes, time.Now().Unix())
		if err != nil {
			return t, fmt.Errorf("telemetry: record log budget drops: %w", err)
		}
	}
	return t, nil
}
