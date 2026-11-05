package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrUserPreferenceTooLarge is returned when a value is over the size cap.
var ErrUserPreferenceTooLarge = errors.New("store: user preference is too large")

// GetUserPreference returns one preference of the user; found is false when unset.
func (db *DB) GetUserPreference(ctx context.Context, userID, key string) (value string, found bool, err error) {
	err = db.QueryRowContext(ctx, `SELECT value FROM user_preference_docs WHERE user_id = ? AND key = ?`, userID, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: get preference %q for %q: %w", key, userID, err)
	}
	return value, true, nil
}

// SetUserPreference upserts a preference; maxBytes <= 0 means no cap.
func (db *DB) SetUserPreference(ctx context.Context, userID, key, value string, maxBytes int) error {
	if maxBytes > 0 && len(value) > maxBytes {
		return ErrUserPreferenceTooLarge
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_preference_docs (user_id, key, value, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		userID, key, value, formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("store: save preference %q for %q: %w", key, userID, err)
	}
	return nil
}

// DeleteUserPreference removes one preference; deleting an unset one is not an error.
func (db *DB) DeleteUserPreference(ctx context.Context, userID, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM user_preference_docs WHERE user_id = ? AND key = ?`, userID, key); err != nil {
		return fmt.Errorf("store: delete preference %q for %q: %w", key, userID, err)
	}
	return nil
}
