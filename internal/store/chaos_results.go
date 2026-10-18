package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

const chaosResultColumns = `id, scenario, status, summary, recovery_ms, error_window_ms, requests, request_errors, assertions, hardware, duration_ms, ran_at`

// RecordChaosResult stores one measured scenario run and returns its ID.
func (db *DB) RecordChaosResult(ctx context.Context, r chaos.Result, recordedBy string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: chaos result id: %w", err)
	}
	id := "chaos_" + hex.EncodeToString(b[:])
	assertions, err := json.Marshal(r.Assertions)
	if err != nil {
		return "", fmt.Errorf("store: encode chaos assertions: %w", err)
	}
	if r.RanAt.IsZero() {
		r.RanAt = time.Now().UTC()
	}
	_, err = db.ExecContext(ctx, `INSERT INTO chaos_results (`+chaosResultColumns+`, recorded_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, r.Scenario, r.Status, r.Summary, r.RecoveryMS, r.ErrorWindowMS, r.Requests, r.RequestErrors,
		string(assertions), r.Hardware, r.DurationMS, r.RanAt.UTC().Format(time.RFC3339Nano), recordedBy)
	if err != nil {
		return "", fmt.Errorf("store: record chaos result: %w", err)
	}
	return id, nil
}

// ListChaosResults returns the newest results first, at most limit rows.
func (db *DB) ListChaosResults(ctx context.Context, limit int) ([]chaos.Result, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+chaosResultColumns+` FROM chaos_results ORDER BY ran_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list chaos results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []chaos.Result
	for rows.Next() {
		var r chaos.Result
		var assertions, ranAt string
		if err := rows.Scan(&r.ID, &r.Scenario, &r.Status, &r.Summary, &r.RecoveryMS, &r.ErrorWindowMS, &r.Requests, &r.RequestErrors,
			&assertions, &r.Hardware, &r.DurationMS, &ranAt); err != nil {
			return nil, fmt.Errorf("store: scan chaos result: %w", err)
		}
		if err := json.Unmarshal([]byte(assertions), &r.Assertions); err != nil {
			return nil, fmt.Errorf("store: decode chaos assertions: %w", err)
		}
		r.RanAt, _ = time.Parse(time.RFC3339Nano, ranAt)
		out = append(out, r)
	}
	return out, rows.Err()
}
