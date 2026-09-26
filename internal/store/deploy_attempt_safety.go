package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// How a deploy attempt's ImageDigest was obtained.
const (
	// DigestReasonResolved: the registry answered for the tag at deploy time.
	DigestReasonResolved = "Resolved"
	// DigestReasonPinned: the requested reference already carried a digest.
	DigestReasonPinned = "AlreadyPinned"
	// DigestReasonLocalBuild: the image ID of the image this deploy built.
	DigestReasonLocalBuild = "LocalBuild"
	// DigestReasonPullFailedUsingCached: the registry could not be reached,
	// so the locally cached image for the tag was used instead.
	DigestReasonPullFailedUsingCached = "PullFailedUsingCached"
	// DigestReasonUnresolved: neither the registry nor the local cache knew
	// the tag; the node resolves it when it creates the container.
	DigestReasonUnresolved = "Unresolved"
)

// What the application controller observed for an attempt's image.
const (
	RolloutStateServing  = "serving"
	RolloutStateMismatch = "mismatch"
)

// Reasons recorded on held and superseded attempts.
const (
	DeployReasonFrozen     = "Frozen"
	DeployReasonSuperseded = "Superseded"
	DeployReasonStale      = "StaleCommit"
)

// SetDeployAttemptDigest records the content identity an attempt resolved to.
func (db *DB) SetDeployAttemptDigest(ctx context.Context, id, digest, reason string) error {
	res, err := db.ExecContext(ctx, `UPDATE deploy_attempts SET image_digest = ?, digest_reason = ? WHERE id = ?`, digest, reason, id)
	if err != nil {
		return fmt.Errorf("store: set deploy attempt %q digest: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrDeployAttemptNotFound
	}
	return nil
}

// RecordRollout stamps what is actually running onto the newest succeeded
// attempt that deployed image. It is a no-op when nothing changed, so the
// reconciler can call it on every pass without write amplification.
func (db *DB) RecordRollout(ctx context.Context, serviceName, image, state, runningImageID string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET rollout_state = ?, running_image_id = ?
		WHERE id = (
			SELECT id FROM deploy_attempts
			WHERE service_name = ? AND image = ? AND status = ?
			ORDER BY started_at DESC LIMIT 1
		) AND (rollout_state != ? OR running_image_id != ?)
	`, state, runningImageID, serviceName, image, DeployAttemptStatusSucceeded, state, runningImageID)
	if err != nil {
		return fmt.Errorf("store: record rollout for %q: %w", serviceName, err)
	}
	return nil
}

// ListHeldDeployAttempts returns every held attempt, oldest first.
func (db *DB) ListHeldDeployAttempts(ctx context.Context) ([]DeployAttempt, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+deployAttemptColumns+`
		FROM deploy_attempts WHERE status = ?
		ORDER BY started_at ASC
	`, DeployAttemptStatusHeld)
	if err != nil {
		return nil, fmt.Errorf("store: list held deploy attempts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployAttempt
	for rows.Next() {
		a, err := scanDeployAttempt(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan held deploy attempt: %w", err)
		}
		out = append(out, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate held deploy attempts: %w", err)
	}
	return out, nil
}

// TransitionDeployAttempt moves an attempt from one status to another only
// if it is still in from, so two releasers can never both act on it. A
// terminal to status also stamps finished_at. Reports whether it moved.
func (db *DB) TransitionDeployAttempt(ctx context.Context, id, from, to, reason string) (bool, error) {
	var finished sql.NullString
	if to != DeployAttemptStatusRunning && to != DeployAttemptStatusHeld {
		finished = sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true}
	}
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, reason = ?, finished_at = COALESCE(?, finished_at)
		WHERE id = ? AND status = ?
	`, to, reason, finished, id, from)
	if err != nil {
		return false, fmt.Errorf("store: transition deploy attempt %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: transition deploy attempt %q: rows affected: %w", id, err)
	}
	return n > 0, nil
}
