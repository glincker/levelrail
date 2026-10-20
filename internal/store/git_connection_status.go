package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Git connection health values stored in git_connection_status.
const (
	GitConnOK               = "ok"
	GitConnNeedsReauthorize = "needs_reauthorize"
	GitConnRevoked          = "revoked"
	GitConnRateLimited      = "rate_limited"
)

// GitConnectionStatus is one provider connection's last known health.
type GitConnectionStatus struct {
	Provider    string
	Status      string
	Detail      string
	NextCheckAt time.Time
	UpdatedAt   time.Time
}

// SetGitConnectionStatus records the health of a provider connection.
func (db *DB) SetGitConnectionStatus(ctx context.Context, s GitConnectionStatus) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO git_connection_status (provider, status, detail, next_check_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(provider) DO UPDATE SET status = excluded.status, detail = excluded.detail,
			next_check_at = excluded.next_check_at, updated_at = excluded.updated_at`,
		s.Provider, s.Status, s.Detail, formatOptionalTime(s.NextCheckAt), s.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set git connection status for %q: %w", s.Provider, err)
	}
	return nil
}

// GetGitConnectionStatus returns a provider's recorded health; ok is false
// when nothing was ever recorded (treated as healthy by callers).
func (db *DB) GetGitConnectionStatus(ctx context.Context, provider string) (GitConnectionStatus, bool, error) {
	var (
		s          GitConnectionStatus
		next, upAt string
	)
	err := db.QueryRowContext(ctx, `
		SELECT provider, status, detail, next_check_at, updated_at FROM git_connection_status WHERE provider = ?`, provider).
		Scan(&s.Provider, &s.Status, &s.Detail, &next, &upAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GitConnectionStatus{}, false, nil
	}
	if err != nil {
		return GitConnectionStatus{}, false, fmt.Errorf("store: get git connection status for %q: %w", provider, err)
	}
	s.NextCheckAt, s.UpdatedAt = parseOptionalTime(next), parseOptionalTime(upAt)
	return s, true, nil
}

// DeleteGitConnectionStatus forgets a provider's recorded health.
func (db *DB) DeleteGitConnectionStatus(ctx context.Context, provider string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM git_connection_status WHERE provider = ?`, provider); err != nil {
		return fmt.Errorf("store: delete git connection status for %q: %w", provider, err)
	}
	return nil
}
