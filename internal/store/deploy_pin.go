package store

import (
	"context"
	"fmt"
	"time"
)

// PinDeploy marks a deploy attempt as kept by a person (migrations/3285).
// Pinning twice is a no-op, so the first pinner stays on record.
func (db *DB) PinDeploy(ctx context.Context, serviceName, attemptID, by string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO deploy_pins (deploy_attempt_id, service_name, pinned_by, created_at)
		VALUES (?, ?, ?, ?) ON CONFLICT (deploy_attempt_id) DO NOTHING`,
		attemptID, serviceName, by, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: pin deploy %q: %w", attemptID, err)
	}
	return nil
}

// UnpinDeploy releases a pin; unpinning an unpinned deploy is a no-op.
func (db *DB) UnpinDeploy(ctx context.Context, attemptID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM deploy_pins WHERE deploy_attempt_id = ?`, attemptID); err != nil {
		return fmt.Errorf("store: unpin deploy %q: %w", attemptID, err)
	}
	return nil
}

// ListPinnedDeploys returns the pinned attempt ids of one service.
func (db *DB) ListPinnedDeploys(ctx context.Context, serviceName string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT deploy_attempt_id FROM deploy_pins WHERE service_name = ?`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list pinned deploys of %q: %w", serviceName, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan pinned deploy: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}
