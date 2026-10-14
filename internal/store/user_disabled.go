package store

import (
	"context"
	"fmt"
	"time"
)

// Reasons recorded on a disabled account.
const (
	DisabledReasonSCIM        = "scim_deactivated"
	DisabledReasonSCIMDeleted = "scim_deleted"
)

// UserDisabled reports whether the user is disabled (migration 0611).
func (db *DB) UserDisabled(ctx context.Context, userID string) (bool, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM disabled_users WHERE user_id = ?`, userID).Scan(&n); err != nil {
		return false, fmt.Errorf("store: user disabled %q: %w", userID, err)
	}
	return n > 0, nil
}

// DisableUser marks the user disabled with reason, replacing an earlier reason.
func (db *DB) DisableUser(ctx context.Context, userID, reason string, now time.Time) error {
	res, err := db.ExecContext(ctx, `INSERT INTO disabled_users (user_id, reason, disabled_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET reason = excluded.reason`, userID, reason, formatTime(now))
	if err != nil {
		return fmt.Errorf("store: disable user %q: %w", userID, err)
	}
	return rowsAffectedOrNotFound(res, ErrUserNotFound, "disable user %q", userID)
}

// EnableUser clears the disabled marker; enabling an active user is a no-op.
func (db *DB) EnableUser(ctx context.Context, userID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM disabled_users WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("store: enable user %q: %w", userID, err)
	}
	return nil
}
