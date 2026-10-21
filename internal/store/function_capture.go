package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FunctionCaptureEnabled reports whether the function keeps the input of its test invocations.
func (db *DB) FunctionCaptureEnabled(ctx context.Context, name string) (bool, error) {
	var enabled int
	err := db.QueryRowContext(ctx, `SELECT enabled FROM function_capture WHERE service_name = ?`, name).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: read function capture for %q: %w", name, err)
	}
	return enabled != 0, nil
}

// SetFunctionCapture turns input capture on or off for a function.
func (db *DB) SetFunctionCapture(ctx context.Context, name string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO function_capture (service_name, enabled, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
		name, v, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set function capture for %q: %w", name, err)
	}
	return nil
}
