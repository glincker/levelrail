package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrHeldBuildNotFound is returned when no unpublished held build matches.
var ErrHeldBuildNotFound = errors.New("store: held build not found")

// PublishLock is an app's desired-state lock: while it exists, builds finish
// but are not made live (migrations/2580_publish_locks.sql).
type PublishLock struct {
	ServiceName string    `json:"app"`
	Reason      string    `json:"reason,omitempty"`
	LockedBy    string    `json:"locked_by,omitempty"`
	LockedAt    time.Time `json:"locked_at"`
	// PinnedImage is the image live when the lock was taken.
	PinnedImage string `json:"pinned_image,omitempty"`
}

// GetPublishLock returns the app's lock, or nil when it is not locked.
func (db *DB) GetPublishLock(ctx context.Context, name string) (*PublishLock, error) {
	var l PublishLock
	var at string
	err := db.QueryRowContext(ctx, `SELECT service_name, reason, locked_by, locked_at, pinned_image FROM app_publish_locks WHERE service_name = ?`, name).
		Scan(&l.ServiceName, &l.Reason, &l.LockedBy, &at, &l.PinnedImage)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get publish lock %q: %w", name, err)
	}
	if l.LockedAt, err = time.Parse(time.RFC3339Nano, at); err != nil {
		return nil, fmt.Errorf("store: parse publish lock time: %w", err)
	}
	return &l, nil
}

// SetPublishLock locks an app. Locking an already locked app keeps the
// original lock so who and when are not overwritten.
func (db *DB) SetPublishLock(ctx context.Context, l PublishLock) (PublishLock, error) {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO app_publish_locks (service_name, reason, locked_by, locked_at, pinned_image) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (service_name) DO NOTHING
	`, l.ServiceName, l.Reason, l.LockedBy, l.LockedAt.UTC().Format(time.RFC3339Nano), l.PinnedImage); err != nil {
		return PublishLock{}, fmt.Errorf("store: set publish lock %q: %w", l.ServiceName, err)
	}
	cur, err := db.GetPublishLock(ctx, l.ServiceName)
	if err != nil || cur == nil {
		return PublishLock{}, fmt.Errorf("store: reload publish lock %q: %w", l.ServiceName, err)
	}
	return *cur, nil
}

// ClearPublishLock unlocks an app and reports whether it was locked.
func (db *DB) ClearPublishLock(ctx context.Context, name string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM app_publish_locks WHERE service_name = ?`, name)
	if err != nil {
		return false, fmt.Errorf("store: clear publish lock %q: %w", name, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SaveHeldBuild records the desired state a finished build would have made
// live, keyed by its deploy attempt.
func (db *DB) SaveHeldBuild(ctx context.Context, deployID string, svc DesiredService, at time.Time) error {
	raw, err := json.Marshal(svc)
	if err != nil {
		return fmt.Errorf("store: marshal held build %q: %w", deployID, err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO held_builds (deploy_id, service_name, desired_json, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (deploy_id) DO UPDATE SET desired_json = excluded.desired_json
	`, deployID, svc.Name, string(raw), at.UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("store: save held build %q: %w", deployID, err)
	}
	return nil
}

// ListHeldBuildIDs returns the deploy ids of an app's unpublished held builds.
func (db *DB) ListHeldBuildIDs(ctx context.Context, name string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT deploy_id FROM held_builds WHERE service_name = ? AND published_at IS NULL`, name)
	if err != nil {
		return nil, fmt.Errorf("store: list held builds %q: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan held build: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// GetHeldBuild returns the unpublished held build's saved desired state.
func (db *DB) GetHeldBuild(ctx context.Context, deployID string) (*DesiredService, error) {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT desired_json FROM held_builds WHERE deploy_id = ? AND published_at IS NULL`, deployID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrHeldBuildNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get held build %q: %w", deployID, err)
	}
	if heldStaticSite(raw) != nil {
		return nil, ErrHeldBuildNotFound
	}
	var svc DesiredService
	if err := json.Unmarshal([]byte(raw), &svc); err != nil {
		return nil, fmt.Errorf("store: decode held build %q: %w", deployID, err)
	}
	return &svc, nil
}

// MarkHeldBuildPublished retires a held build after its desired state was
// written. It is idempotent so a retry after a crash between the two steps
// converges.
func (db *DB) MarkHeldBuildPublished(ctx context.Context, deployID, by string, at time.Time) error {
	if _, err := db.ExecContext(ctx, `UPDATE held_builds SET published_at = ?, published_by = ? WHERE deploy_id = ? AND published_at IS NULL`,
		at.UTC().Format(time.RFC3339Nano), by, deployID); err != nil {
		return fmt.Errorf("store: mark held build %q published: %w", deployID, err)
	}
	return nil
}
