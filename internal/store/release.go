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

// Release is an immutable snapshot minted from one succeeded deploy attempt
// (migrations/3054). The store exposes insert and read only.
type Release struct {
	ID                    string
	ProjectID             string
	SourceApp             string
	SourceDeployAttemptID string
	ImageRef              string
	ImageTag              string
	// Config is the environment neutral JSON subset, never secret values.
	Config    string
	CommitSHA string
	CreatedAt time.Time
	CreatedBy string
}

// EnvironmentBinding records which release an app's environment is on.
type EnvironmentBinding struct {
	App               string
	EnvironmentID     string
	ReleaseID         string
	State             string
	PreviousReleaseID string
	UpdatedAt         time.Time
	UpdatedBy         string
}

// BindingHistoryEntry is one append only retarget record.
type BindingHistoryEntry struct {
	App        string
	ReleaseID  string
	ChangedAt  time.Time
	ChangedBy  string
	Reason     string
	ApprovalID string
}

// BindingStateActive is the only binding state written in the shadow phase.
const BindingStateActive = "active"

// NewReleaseID mints an opaque release identifier.
func NewReleaseID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: mint release id: %w", err)
	}
	return "rel_" + hex.EncodeToString(b), nil
}

// InsertRelease stores r unless its attempt already has a release. It
// reports whether a row was created, which keeps the writer idempotent.
func (db *DB) InsertRelease(ctx context.Context, r Release) (bool, error) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.Config == "" {
		r.Config = "{}"
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO releases (id, project_id, source_app, source_deploy_attempt_id, image_ref, image_tag, config, commit_sha, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source_deploy_attempt_id) DO NOTHING
	`, r.ID, r.ProjectID, r.SourceApp, r.SourceDeployAttemptID, r.ImageRef, r.ImageTag, r.Config, r.CommitSHA, r.CreatedAt.UTC().Format(time.RFC3339Nano), r.CreatedBy)
	if err != nil {
		return false, fmt.Errorf("store: insert release for attempt %q: %w", r.SourceDeployAttemptID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: insert release for attempt %q: rows affected: %w", r.SourceDeployAttemptID, err)
	}
	return n > 0, nil
}

// GetReleaseByAttempt returns the release minted from attemptID, or nil.
func (db *DB) GetReleaseByAttempt(ctx context.Context, attemptID string) (*Release, error) {
	rels, err := db.queryReleases(ctx, `WHERE source_deploy_attempt_id = ?`, attemptID)
	if err != nil || len(rels) == 0 {
		return nil, err
	}
	return &rels[0], nil
}

// ListReleasesByApp returns an app's releases, newest first.
func (db *DB) ListReleasesByApp(ctx context.Context, app string) ([]Release, error) {
	return db.queryReleases(ctx, `WHERE source_app = ? ORDER BY created_at DESC, id DESC`, app)
}

func (db *DB) queryReleases(ctx context.Context, where string, args ...any) ([]Release, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, project_id, source_app, source_deploy_attempt_id, image_ref, image_tag, config, commit_sha, created_at, created_by
		FROM releases `+where, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query releases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Release
	for rows.Next() {
		var r Release
		var created string
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.SourceApp, &r.SourceDeployAttemptID, &r.ImageRef, &r.ImageTag, &r.Config, &r.CommitSHA, &created, &r.CreatedBy); err != nil {
			return nil, fmt.Errorf("store: scan release: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordShadowBinding points app's binding at releaseID and appends a
// history row, in one transaction. A repeat for the current release is a
// no-op, so replays and out of order hooks stay idempotent.
func (db *DB) RecordShadowBinding(ctx context.Context, app, environmentID, releaseID, reason, by string) (bool, error) {
	return db.RetargetBinding(ctx, app, environmentID, releaseID, reason, by, "")
}

// RetargetBinding is RecordShadowBinding that also names the approval that
// authorized the move, so history can answer who allowed a release.
func (db *DB) RetargetBinding(ctx context.Context, app, environmentID, releaseID, reason, by, approvalID string) (bool, error) {
	return db.retargetBinding(ctx, app, environmentID, releaseID, reason, by, approvalID, nil)
}

// RetargetBindingIf is RetargetBinding that only writes while the binding
// still names expected, so an undo never overwrites a newer retarget.
func (db *DB) RetargetBindingIf(ctx context.Context, app, environmentID, releaseID, expected, reason, by, approvalID string) (bool, error) {
	return db.retargetBinding(ctx, app, environmentID, releaseID, reason, by, approvalID, &expected)
}

func (db *DB) retargetBinding(ctx context.Context, app, environmentID, releaseID, reason, by, approvalID string, expected *string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: begin shadow binding: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT release_id FROM environment_bindings WHERE app = ?`, app).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("store: read binding %q: %w", app, err)
	}
	if expected != nil && current.String != *expected {
		return false, nil
	}
	if current.Valid && current.String == releaseID {
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO environment_bindings (app, environment_id, release_id, state, previous_release_id, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (app) DO UPDATE SET environment_id = excluded.environment_id, release_id = excluded.release_id,
			previous_release_id = excluded.previous_release_id, updated_at = excluded.updated_at, updated_by = excluded.updated_by
	`, app, environmentID, releaseID, BindingStateActive, current, now, by); err != nil {
		return false, fmt.Errorf("store: write binding %q: %w", app, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO environment_binding_history (app, release_id, changed_at, changed_by, reason, approval_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, app, releaseID, now, by, reason, approvalID); err != nil {
		return false, fmt.Errorf("store: append binding history %q: %w", app, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: commit shadow binding %q: %w", app, err)
	}
	return true, nil
}

// UnbindRelease clears app's binding after a failed first deploy, but only
// while it still names expected. The history row records which release was
// removed, with reason "unbound".
func (db *DB) UnbindRelease(ctx context.Context, app, expected, by string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("store: begin unbind %q: %w", app, err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := tx.ExecContext(ctx, `UPDATE environment_bindings SET release_id = NULL, updated_at = ?, updated_by = ? WHERE app = ? AND release_id = ?`, now, by, app, expected)
	if err != nil {
		return false, fmt.Errorf("store: unbind release %q: %w", app, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO environment_binding_history (app, release_id, changed_at, changed_by, reason) VALUES (?, ?, ?, ?, 'unbound')`, app, expected, now, by); err != nil {
		return false, fmt.Errorf("store: append unbind history %q: %w", app, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("store: commit unbind %q: %w", app, err)
	}
	return true, nil
}

// GetRelease returns the release with this id, or nil when none exists.
func (db *DB) GetRelease(ctx context.Context, id string) (*Release, error) {
	rels, err := db.queryReleases(ctx, `WHERE id = ?`, id)
	if err != nil || len(rels) == 0 {
		return nil, err
	}
	return &rels[0], nil
}

// GetEnvironmentBinding returns app's binding, or nil when none exists.
func (db *DB) GetEnvironmentBinding(ctx context.Context, app string) (*EnvironmentBinding, error) {
	var b EnvironmentBinding
	var rel, prev sql.NullString
	var updated string
	err := db.QueryRowContext(ctx, `
		SELECT app, environment_id, release_id, state, previous_release_id, updated_at, updated_by
		FROM environment_bindings WHERE app = ?`, app).Scan(&b.App, &b.EnvironmentID, &rel, &b.State, &prev, &updated, &b.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get binding %q: %w", app, err)
	}
	b.ReleaseID, b.PreviousReleaseID = rel.String, prev.String
	b.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &b, nil
}

// ListBindingHistory returns app's history, oldest first.
func (db *DB) ListBindingHistory(ctx context.Context, app string) ([]BindingHistoryEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT app, release_id, changed_at, changed_by, reason, approval_id
		FROM environment_binding_history WHERE app = ? ORDER BY id`, app)
	if err != nil {
		return nil, fmt.Errorf("store: list binding history %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	var out []BindingHistoryEntry
	for rows.Next() {
		var h BindingHistoryEntry
		var at string
		if err := rows.Scan(&h.App, &h.ReleaseID, &at, &h.ChangedBy, &h.Reason, &h.ApprovalID); err != nil {
			return nil, fmt.Errorf("store: scan binding history: %w", err)
		}
		h.ChangedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, h)
	}
	return out, rows.Err()
}

// AttemptObserver is told when a deploy attempt succeeds or gets its digest.
// It must not fail the caller, so it returns nothing.
type AttemptObserver func(ctx context.Context, attemptID string)

// SetAttemptObserver installs obs; nil removes it.
func (db *DB) SetAttemptObserver(obs AttemptObserver) {
	if obs == nil {
		db.attemptObs.Store(nil)
		return
	}
	db.attemptObs.Store(&obs)
}

func (db *DB) notifyAttempt(ctx context.Context, id string) {
	if p := db.attemptObs.Load(); p != nil {
		(*p)(ctx, id)
	}
}
