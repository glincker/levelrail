package store

import (
	"context"
	"fmt"
	"time"
)

// Audit actions for critical announcement handling, so a snooze or dismissal
// of a security notice is traceable to the user who did it.
const (
	AuditActionAnnouncementDismissed = "announcement.dismissed"
	AuditActionAnnouncementSnoozed   = "announcement.snoozed"
)

// AnnouncementFeedState is the singleton row describing the last accepted feed
// and the outcome of the latest fetch.
type AnnouncementFeedState struct {
	LastSequence int64
	ETag         string
	FetchedAt    string
	VerifiedAt   string
	ExpiresAt    string
	LastError    string
}

// AnnouncementRow is one verified item as cached; the payload is the item JSON.
type AnnouncementRow struct {
	ID          string
	Tier        string
	Kind        string
	PayloadJSON string
	IssuedAt    string
	ExpiresAt   string
	Sequence    int64
}

// AnnouncementUserState is one user's read and snooze marks for an item.
type AnnouncementUserState struct {
	ReadAt       string
	SnoozedUntil string
}

// GetAnnouncementFeedState returns the singleton state row.
func (db *DB) GetAnnouncementFeedState(ctx context.Context) (AnnouncementFeedState, error) {
	var s AnnouncementFeedState
	err := db.reader().QueryRowContext(ctx, `
		SELECT last_sequence, etag, fetched_at, verified_at, expires_at, last_error
		FROM announcement_feed_state WHERE id = 1`).
		Scan(&s.LastSequence, &s.ETag, &s.FetchedAt, &s.VerifiedAt, &s.ExpiresAt, &s.LastError)
	if err != nil {
		return s, fmt.Errorf("store: get announcement feed state: %w", err)
	}
	return s, nil
}

// RecordAnnouncementFetch updates only the fetch outcome, keeping the accepted
// sequence and cache. A non-empty etag replaces the stored one.
func (db *DB) RecordAnnouncementFetch(ctx context.Context, etag, fetchedAt, lastError string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE announcement_feed_state
		SET etag = CASE WHEN ? <> '' THEN ? ELSE etag END, fetched_at = ?, last_error = ?
		WHERE id = 1`, etag, etag, fetchedAt, lastError)
	if err != nil {
		return fmt.Errorf("store: record announcement fetch: %w", err)
	}
	return nil
}

// SaveAnnouncementFeed replaces the cache with a verified feed, advances the
// sequence and records newly revoked keys in one transaction.
func (db *DB) SaveAnnouncementFeed(ctx context.Context, st AnnouncementFeedState, rows []AnnouncementRow, revoked []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save announcement feed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM announcements`); err != nil {
		return fmt.Errorf("store: save announcement feed: clear: %w", err)
	}
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO announcements (id, tier, kind, payload_json, issued_at, expires_at, seen_sequence)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, r.ID, r.Tier, r.Kind, r.PayloadJSON, r.IssuedAt, r.ExpiresAt, r.Sequence); err != nil {
			return fmt.Errorf("store: save announcement %q: %w", r.ID, err)
		}
	}
	for _, k := range revoked {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO announcement_revoked_keys (key_id, revoked_at, sequence) VALUES (?, ?, ?)`,
			k, st.VerifiedAt, st.LastSequence); err != nil {
			return fmt.Errorf("store: save revoked key %q: %w", k, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE announcement_feed_state
		SET last_sequence = ?, etag = ?, fetched_at = ?, verified_at = ?, expires_at = ?, last_error = ''
		WHERE id = 1`, st.LastSequence, st.ETag, st.FetchedAt, st.VerifiedAt, st.ExpiresAt); err != nil {
		return fmt.Errorf("store: save announcement feed: state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save announcement feed: commit: %w", err)
	}
	return nil
}

// ListAnnouncements returns the cached verified items.
func (db *DB) ListAnnouncements(ctx context.Context) ([]AnnouncementRow, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT id, tier, kind, payload_json, issued_at, expires_at, seen_sequence
		FROM announcements ORDER BY issued_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list announcements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AnnouncementRow
	for rows.Next() {
		var r AnnouncementRow
		if err := rows.Scan(&r.ID, &r.Tier, &r.Kind, &r.PayloadJSON, &r.IssuedAt, &r.ExpiresAt, &r.Sequence); err != nil {
			return nil, fmt.Errorf("store: scan announcement: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListRevokedAnnouncementKeys returns the key ids a signed feed revoked.
func (db *DB) ListRevokedAnnouncementKeys(ctx context.Context) ([]string, error) {
	rows, err := db.reader().QueryContext(ctx, `SELECT key_id FROM announcement_revoked_keys`)
	if err != nil {
		return nil, fmt.Errorf("store: list revoked announcement keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("store: scan revoked key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ListAnnouncementUserState returns one user's marks keyed by announcement id.
func (db *DB) ListAnnouncementUserState(ctx context.Context, userID string) (map[string]AnnouncementUserState, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT announcement_id, read_at, snoozed_until FROM announcement_user_state WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list announcement user state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]AnnouncementUserState{}
	for rows.Next() {
		var id string
		var s AnnouncementUserState
		if err := rows.Scan(&id, &s.ReadAt, &s.SnoozedUntil); err != nil {
			return nil, fmt.Errorf("store: scan announcement user state: %w", err)
		}
		out[id] = s
	}
	return out, rows.Err()
}

// MarkAnnouncementRead records that the user opened an item.
func (db *DB) MarkAnnouncementRead(ctx context.Context, userID, id string, at time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO announcement_user_state (user_id, announcement_id, read_at) VALUES (?, ?, ?)
		ON CONFLICT (user_id, announcement_id) DO UPDATE SET read_at = excluded.read_at`,
		userID, id, FormatAuditTime(at))
	if err != nil {
		return fmt.Errorf("store: mark announcement read: %w", err)
	}
	return nil
}

// SnoozeAnnouncement hides an item for the user until the given time and, when
// entry is set, writes its audit row in the same transaction.
func (db *DB) SnoozeAnnouncement(ctx context.Context, userID, id string, until time.Time, entry *AuditEntry) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: snooze announcement: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO announcement_user_state (user_id, announcement_id, snoozed_until) VALUES (?, ?, ?)
		ON CONFLICT (user_id, announcement_id) DO UPDATE SET snoozed_until = excluded.snoozed_until`,
		userID, id, FormatAuditTime(until)); err != nil {
		return fmt.Errorf("store: snooze announcement: %w", err)
	}
	if entry != nil {
		if err := insertAuditEntry(ctx, tx, *entry); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: snooze announcement: commit: %w", err)
	}
	return nil
}
