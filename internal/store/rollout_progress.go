package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SaveRolloutProgress replaces serviceName's last rollout progress. payload
// is opaque JSON owned by the rollout package.
func (db *DB) SaveRolloutProgress(ctx context.Context, serviceName string, payload []byte) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO rollout_progress (service_name, payload, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(service_name) DO UPDATE SET payload = excluded.payload, updated_at = excluded.updated_at
	`, serviceName, string(payload), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save rollout progress for %q: %w", serviceName, err)
	}
	return nil
}

// LoadRolloutProgress returns serviceName's saved progress payload.
func (db *DB) LoadRolloutProgress(ctx context.Context, serviceName string) ([]byte, bool, error) {
	var payload string
	err := db.QueryRowContext(ctx, `SELECT payload FROM rollout_progress WHERE service_name = ?`, serviceName).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: load rollout progress for %q: %w", serviceName, err)
	}
	return []byte(payload), true, nil
}
