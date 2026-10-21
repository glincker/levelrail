package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Webhook registration states stored in git_source_reliability.
const (
	GitWebhookRegistered = "registered"
	GitWebhookFailed     = "failed"
)

// GitSourceReliability is the webhook and poll bookkeeping of one git
// source. The zero value means nothing was ever recorded.
type GitSourceReliability struct {
	ServiceName string
	// WebhookState is registered, failed, or empty when the control plane
	// never tried (a manually added webhook).
	WebhookState     string
	WebhookError     string
	WebhookChangedAt time.Time
	LastWebhookAt    time.Time
	// LastSeenHead is the newest branch head a webhook or the poller saw.
	LastSeenHead string
	LastSeenAt   time.Time
	LastPolledAt time.Time
	PollError    string
}

func (db *DB) ensureGitSourceReliability(ctx context.Context, name string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO git_source_reliability (service_name)
		SELECT service_name FROM service_git_sources WHERE service_name = ?
		ON CONFLICT (service_name) DO NOTHING`, name)
	if err != nil {
		return fmt.Errorf("store: ensure git source reliability for %q: %w", name, err)
	}
	return nil
}

// GetGitSourceReliability returns the bookkeeping for name, the zero value when none exists.
func (db *DB) GetGitSourceReliability(ctx context.Context, name string) (GitSourceReliability, error) {
	var (
		r                                      GitSourceReliability
		changed, lastWebhook, seenAt, polledAt string
	)
	err := db.QueryRowContext(ctx, `
		SELECT service_name, webhook_state, webhook_error, webhook_changed_at, last_webhook_at, last_seen_head, last_seen_at, last_polled_at, poll_error
		FROM git_source_reliability WHERE service_name = ?`, name).
		Scan(&r.ServiceName, &r.WebhookState, &r.WebhookError, &changed, &lastWebhook, &r.LastSeenHead, &seenAt, &polledAt, &r.PollError)
	if errors.Is(err, sql.ErrNoRows) {
		return GitSourceReliability{ServiceName: name}, nil
	}
	if err != nil {
		return GitSourceReliability{}, fmt.Errorf("store: get git source reliability for %q: %w", name, err)
	}
	r.WebhookChangedAt, r.LastWebhookAt = parseOptionalTime(changed), parseOptionalTime(lastWebhook)
	r.LastSeenAt, r.LastPolledAt = parseOptionalTime(seenAt), parseOptionalTime(polledAt)
	return r, nil
}

// SetGitSourceWebhookState records the outcome of registering name's webhook on the provider.
func (db *DB) SetGitSourceWebhookState(ctx context.Context, name, state, errMsg string, at time.Time) error {
	if err := db.ensureGitSourceReliability(ctx, name); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE git_source_reliability SET webhook_state = ?, webhook_error = ?, webhook_changed_at = ? WHERE service_name = ?`,
		state, errMsg, formatOptionalTime(at), name); err != nil {
		return fmt.Errorf("store: set git source webhook state for %q: %w", name, err)
	}
	return nil
}

// NoteGitSourceHead records the newest branch head seen for name; webhook is
// true when it arrived through a webhook delivery.
func (db *DB) NoteGitSourceHead(ctx context.Context, name, head string, at time.Time, webhook bool) error {
	if err := db.ensureGitSourceReliability(ctx, name); err != nil {
		return err
	}
	q := `UPDATE git_source_reliability SET last_seen_head = ?, last_seen_at = ? WHERE service_name = ?`
	args := []any{head, formatOptionalTime(at), name}
	if webhook {
		q = `UPDATE git_source_reliability SET last_seen_head = ?, last_seen_at = ?, last_webhook_at = ? WHERE service_name = ?`
		args = []any{head, formatOptionalTime(at), formatOptionalTime(at), name}
	}
	if _, err := db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("store: note git source head for %q: %w", name, err)
	}
	return nil
}

// SetGitSourcePollResult records when name was last polled and why it failed, if it did.
func (db *DB) SetGitSourcePollResult(ctx context.Context, name string, at time.Time, pollErr string) error {
	if err := db.ensureGitSourceReliability(ctx, name); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE git_source_reliability SET last_polled_at = ?, poll_error = ? WHERE service_name = ?`,
		formatOptionalTime(at), pollErr, name); err != nil {
		return fmt.Errorf("store: set git source poll result for %q: %w", name, err)
	}
	return nil
}
