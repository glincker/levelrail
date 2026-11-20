package pages

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SQLExecer is the slice of *sql.DB the store needs; the alerting DB satisfies it.
type SQLExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// NotifyStore persists NotifyConfig per resource ID.
type NotifyStore struct{ db SQLExecer }

// NewNotifyStore wraps db, which must hold the pages_notify_settings table.
func NewNotifyStore(db SQLExecer) *NotifyStore { return &NotifyStore{db: db} }

// Get returns the stored config and whether a row exists.
func (s *NotifyStore) Get(ctx context.Context, resourceID string) (NotifyConfig, bool, error) {
	var en, ok, fail int
	err := s.db.QueryRowContext(ctx,
		`SELECT enabled, on_success, on_failure FROM pages_notify_settings WHERE resource_id = ?`, resourceID,
	).Scan(&en, &ok, &fail)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultNotifyConfig(), false, nil
	}
	if err != nil {
		return NotifyConfig{}, false, fmt.Errorf("pages: read notify settings: %w", err)
	}
	return NotifyConfig{Enabled: en != 0, OnSuccess: ok != 0, OnFailure: fail != 0}, true, nil
}

// Put creates or replaces the config.
func (s *NotifyStore) Put(ctx context.Context, resourceID string, c NotifyConfig) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pages_notify_settings (resource_id, enabled, on_success, on_failure, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (resource_id) DO UPDATE SET
			enabled = excluded.enabled, on_success = excluded.on_success,
			on_failure = excluded.on_failure, updated_at = excluded.updated_at`,
		resourceID, b2i(c.Enabled), b2i(c.OnSuccess), b2i(c.OnFailure), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("pages: save notify settings: %w", err)
	}
	return nil
}

// Delete drops the row so the site returns to the default.
func (s *NotifyStore) Delete(ctx context.Context, resourceID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM pages_notify_settings WHERE resource_id = ?`, resourceID); err != nil {
		return fmt.Errorf("pages: delete notify settings: %w", err)
	}
	return nil
}

// AllowDeployOutcome lets the deploy dispatcher skip filtered events; a read error fails open.
func (s *NotifyStore) AllowDeployOutcome(ctx context.Context, resourceID string, succeeded bool) bool {
	c, _, err := s.Get(ctx, resourceID)
	if err != nil {
		return true
	}
	return c.Allows(succeeded)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
