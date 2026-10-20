package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const pushSubscriptionIDPrefix = "push_"

// NewPushSubscriptionID mints an opaque push subscription ID, the same
// shape NewAppEventID already establishes.
func NewPushSubscriptionID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("store: mint push subscription id: %w", err)
	}
	return pushSubscriptionIDPrefix + hex.EncodeToString(b[:]), nil
}

// PushSubscription is one browser's Web Push registration
// (migrations/0271), scoped to the account that registered it. Endpoint
// is globally unique (a push service mints one per subscribe call), so
// re-subscribing the same browser upserts rather than duplicating a row.
type PushSubscription struct {
	ID         string
	UserID     string
	Endpoint   string
	P256dh     string
	Auth       string
	UserAgent  string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// ErrPushSubscriptionNotFound is returned by DeletePushSubscription when
// no row matches (id, userID).
var ErrPushSubscriptionNotFound = errors.New("store: push subscription not found")

// SavePushSubscription inserts a new subscription, or replaces the
// existing row for the same endpoint: a browser re-subscribing (a
// permission reset, a cleared service worker) must overwrite its own
// stale keys rather than accumulate a dead duplicate alongside them.
func (db *DB) SavePushSubscription(ctx context.Context, s PushSubscription) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id    = excluded.user_id,
			p256dh     = excluded.p256dh,
			auth       = excluded.auth,
			user_agent = excluded.user_agent
	`, s.ID, s.UserID, s.Endpoint, s.P256dh, s.Auth, s.UserAgent, formatTime(s.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: save push subscription %q: %w", s.ID, err)
	}
	return nil
}

// ListPushSubscriptionsForUser returns every subscription userID has
// registered, oldest first: the settings-page listing.
func (db *DB) ListPushSubscriptionsForUser(ctx context.Context, userID string) ([]PushSubscription, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_used_at
		FROM push_subscriptions WHERE user_id = ? ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list push subscriptions for user %q: %w", userID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PushSubscription
	for rows.Next() {
		s, err := scanPushSubscription(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan push subscription row: %w", err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate push subscription rows: %w", err)
	}
	return out, nil
}

// ListPushSubscriptions returns every registered subscription across
// every user: the "webpush" notification-channel kind's own send fan-out
// (internal/webpush), which has no per-rule or per-app destination to
// scope by, unlike every URL-based channel kind.
func (db *DB) ListPushSubscriptions(ctx context.Context) ([]PushSubscription, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at, last_used_at
		FROM push_subscriptions ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list push subscriptions: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PushSubscription
	for rows.Next() {
		s, err := scanPushSubscription(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan push subscription row: %w", err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate push subscription rows: %w", err)
	}
	return out, nil
}

// DeletePushSubscription removes subscription id, scoped to userID so an
// account can only ever revoke its own registrations.
func (db *DB) DeletePushSubscription(ctx context.Context, id, userID string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("store: delete push subscription %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrPushSubscriptionNotFound, "delete push subscription %q", id)
}

// DeletePushSubscriptionByID removes subscription id regardless of
// owner: internal/webpush's own send loop prunes a subscription a push
// service reports permanently gone (404/410), which has no session user
// to scope the delete to.
func (db *DB) DeletePushSubscriptionByID(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete push subscription %q: %w", id, err)
	}
	return nil
}

func scanPushSubscription(scan func(dest ...any) error) (*PushSubscription, error) {
	var (
		s          PushSubscription
		createdAt  string
		lastUsedAt sql.NullString
	)
	if err := scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth, &s.UserAgent, &createdAt, &lastUsedAt); err != nil {
		return nil, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	s.CreatedAt = parsed
	last, err := parseTimePtr(lastUsedAt)
	if err != nil {
		return nil, fmt.Errorf("parse last_used_at: %w", err)
	}
	s.LastUsedAt = last
	return &s, nil
}
