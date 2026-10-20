package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RollbackPin is an image kept tagged as a rollback target for one
// succeeded deploy attempt (migrations/0461_rollback_pins.sql).
type RollbackPin struct {
	ServiceName     string
	DeployAttemptID string
	Image           string
	ImageID         string
	// PinRef is the extra tag that keeps ImageID from ever becoming dangling.
	PinRef    string
	CreatedAt time.Time
}

// SaveRollbackPin records p, replacing any earlier pin for the same attempt.
func (db *DB) SaveRollbackPin(ctx context.Context, p RollbackPin) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO rollback_pins (service_name, deploy_attempt_id, image, image_id, pin_ref, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, deploy_attempt_id) DO UPDATE SET
			image = excluded.image, image_id = excluded.image_id, pin_ref = excluded.pin_ref
	`, p.ServiceName, p.DeployAttemptID, p.Image, p.ImageID, p.PinRef, p.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save rollback pin for %q: %w", p.ServiceName, err)
	}
	return nil
}

func (db *DB) queryRollbackPins(ctx context.Context, what, where string, args ...any) ([]RollbackPin, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, deploy_attempt_id, image, image_id, pin_ref, created_at
		FROM rollback_pins WHERE `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	var out []RollbackPin
	for rows.Next() {
		var (
			p       RollbackPin
			created string
		)
		if err := rows.Scan(&p.ServiceName, &p.DeployAttemptID, &p.Image, &p.ImageID, &p.PinRef, &created); err != nil {
			return nil, fmt.Errorf("store: %s: scan: %w", what, err)
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: %s: %w", what, err)
	}
	return out, nil
}

// ListRollbackPins returns serviceName's pins, newest first.
func (db *DB) ListRollbackPins(ctx context.Context, serviceName string) ([]RollbackPin, error) {
	return db.queryRollbackPins(ctx, "list rollback pins", "service_name = ? ORDER BY created_at DESC, deploy_attempt_id DESC", serviceName)
}

// GetRollbackPin returns the pin for attemptID, or nil when it has none.
func (db *DB) GetRollbackPin(ctx context.Context, serviceName, attemptID string) (*RollbackPin, error) {
	pins, err := db.queryRollbackPins(ctx, "get rollback pin", "service_name = ? AND deploy_attempt_id = ?", serviceName, attemptID)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, nil
	}
	return &pins[0], nil
}

// IsImagePinned reports whether any rollback pin holds imageID.
func (db *DB) IsImagePinned(ctx context.Context, imageID string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM rollback_pins WHERE image_id = ? LIMIT 1`, imageID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check image pinned: %w", err)
	}
	return true, nil
}

// DeleteRollbackPin removes the pin for attemptID.
func (db *DB) DeleteRollbackPin(ctx context.Context, serviceName, attemptID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM rollback_pins WHERE service_name = ? AND deploy_attempt_id = ?`, serviceName, attemptID); err != nil {
		return fmt.Errorf("store: delete rollback pin for %q: %w", serviceName, err)
	}
	return nil
}
