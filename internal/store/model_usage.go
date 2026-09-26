package store

import (
	"context"
	"fmt"
	"time"
)

// ModelUsage is the gateway usage of one model and key in one hour.
// Token counts only cover requests whose response carried a usage object
// (UsageRequests); the rest add to Requests alone.
type ModelUsage struct {
	ModelName     string
	KeyID         string
	HourStart     time.Time
	Requests      int64
	Status2xx     int64
	Status4xx     int64
	Status5xx     int64
	RateLimited   int64
	UsageRequests int64
	InputTokens   int64
	OutputTokens  int64
	BytesOut      int64
	DurationMsSum int64
	TTFTMsSum     int64
	TTFTCount     int64
}

// AddModelUsage adds the deltas to their hourly rows in one transaction.
func (db *DB) AddModelUsage(ctx context.Context, rows []ModelUsage) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin add model usage: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, u := range rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model_usage_hourly (model_name, key_id, hour_start, requests, status_2xx, status_4xx, status_5xx, rate_limited,
				usage_requests, input_tokens, output_tokens, bytes_out, duration_ms_sum, ttft_ms_sum, ttft_count)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (model_name, key_id, hour_start) DO UPDATE SET
				requests = requests + excluded.requests, status_2xx = status_2xx + excluded.status_2xx,
				status_4xx = status_4xx + excluded.status_4xx, status_5xx = status_5xx + excluded.status_5xx,
				rate_limited = rate_limited + excluded.rate_limited, usage_requests = usage_requests + excluded.usage_requests,
				input_tokens = input_tokens + excluded.input_tokens, output_tokens = output_tokens + excluded.output_tokens,
				bytes_out = bytes_out + excluded.bytes_out, duration_ms_sum = duration_ms_sum + excluded.duration_ms_sum,
				ttft_ms_sum = ttft_ms_sum + excluded.ttft_ms_sum, ttft_count = ttft_count + excluded.ttft_count`,
			u.ModelName, u.KeyID, u.HourStart.Unix(), u.Requests, u.Status2xx, u.Status4xx, u.Status5xx, u.RateLimited,
			u.UsageRequests, u.InputTokens, u.OutputTokens, u.BytesOut, u.DurationMsSum, u.TTFTMsSum, u.TTFTCount)
		if err != nil {
			return fmt.Errorf("store: add usage of model %q: %w", u.ModelName, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit add model usage: %w", err)
	}
	return nil
}

// ListModelUsage returns the hourly rows of a model whose hour lies in
// [from, to), oldest first.
func (db *DB) ListModelUsage(ctx context.Context, model string, from, to time.Time) ([]ModelUsage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT model_name, key_id, hour_start, requests, status_2xx, status_4xx, status_5xx, rate_limited,
			usage_requests, input_tokens, output_tokens, bytes_out, duration_ms_sum, ttft_ms_sum, ttft_count
		FROM model_usage_hourly WHERE model_name = ? AND hour_start >= ? AND hour_start < ? ORDER BY hour_start, key_id`,
		model, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("store: list usage of model %q: %w", model, err)
	}
	defer func() { _ = rows.Close() }()
	var out []ModelUsage
	for rows.Next() {
		var u ModelUsage
		var hour int64
		if err := rows.Scan(&u.ModelName, &u.KeyID, &hour, &u.Requests, &u.Status2xx, &u.Status4xx, &u.Status5xx, &u.RateLimited,
			&u.UsageRequests, &u.InputTokens, &u.OutputTokens, &u.BytesOut, &u.DurationMsSum, &u.TTFTMsSum, &u.TTFTCount); err != nil {
			return nil, fmt.Errorf("store: scan model usage: %w", err)
		}
		u.HourStart = time.Unix(hour, 0).UTC()
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate model usage: %w", err)
	}
	return out, nil
}

// PruneModelUsage deletes hourly rows older than before and reports how many.
func (db *DB) PruneModelUsage(ctx context.Context, before time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM model_usage_hourly WHERE hour_start < ?`, before.Unix())
	if err != nil {
		return 0, fmt.Errorf("store: prune model usage: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
