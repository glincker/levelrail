package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FunctionSettings are one function's request limits (migrations/2740_function_settings.sql).
// A zero field means the server default applies.
type FunctionSettings struct {
	ServiceName           string
	RequestTimeoutSeconds int
	MaxConcurrency        int
}

// GetFunctionSettings returns the stored limits, or zeros when none were set.
func (db *DB) GetFunctionSettings(ctx context.Context, serviceName string) (FunctionSettings, error) {
	s := FunctionSettings{ServiceName: serviceName}
	err := db.QueryRowContext(ctx, `SELECT request_timeout_seconds, max_concurrency FROM function_settings WHERE service_name = ?`, serviceName).
		Scan(&s.RequestTimeoutSeconds, &s.MaxConcurrency)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return FunctionSettings{}, fmt.Errorf("store: get function settings for %q: %w", serviceName, err)
	}
	return s, nil
}

// SetFunctionSettings stores the limits, replacing any earlier row.
func (db *DB) SetFunctionSettings(ctx context.Context, s FunctionSettings, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO function_settings (service_name, request_timeout_seconds, max_concurrency, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(service_name) DO UPDATE SET request_timeout_seconds = excluded.request_timeout_seconds,
  max_concurrency = excluded.max_concurrency, updated_at = excluded.updated_at`,
		s.ServiceName, s.RequestTimeoutSeconds, s.MaxConcurrency, now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set function settings for %q: %w", s.ServiceName, err)
	}
	return nil
}
