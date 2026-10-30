package store

import (
	"context"
	"database/sql"
	"errors"
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
	// DigestReasonLocalImage: the image exists only locally, with no registry digest.
	DigestReasonLocalImage = "LocalImage"
	// DigestReasonStaticTree: the digest hashes the published file tree of a
	// container-less static site.
	DigestReasonStaticTree = "StaticTree"
	// DigestReasonUnresolved: neither the registry nor the local cache knew
	// the tag; the node resolves it when it creates the container.
	DigestReasonUnresolved = "Unresolved"
)

// What the application controller observed for an attempt's image.
const (
	RolloutStateServing  = "serving"
	RolloutStateMismatch = "mismatch"
	RolloutStateFailed   = "failed"
	// RolloutStateSuperseded marks a once-serving attempt that a newer one replaced.
	RolloutStateSuperseded = "superseded"
)

// Reasons recorded on held and superseded attempts.
const (
	DeployReasonFrozen     = "Frozen"
	DeployReasonSuperseded = "Superseded"
	DeployReasonStale      = "StaleCommit"
	// DeployReasonPolled marks a deploy the poller started for a missed webhook.
	DeployReasonPolled = "DetectedByPoll"
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
	db.notifyAttempt(ctx, id)
	return nil
}

// SetDeployAttemptRollout records the rollout state of one attempt by id,
// for deploys with no container for the application controller to observe.
func (db *DB) SetDeployAttemptRollout(ctx context.Context, id, state string) error {
	res, err := db.ExecContext(ctx, `UPDATE deploy_attempts SET rollout_state = ? WHERE id = ?`, state, id)
	if err != nil {
		return fmt.Errorf("store: set deploy attempt %q rollout: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrDeployAttemptNotFound
	}
	if state == RolloutStateServing {
		if err := db.supersedeServingBefore(ctx, id); err != nil {
			return err
		}
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
	if state != RolloutStateServing {
		return nil
	}
	var id string
	err = db.QueryRowContext(ctx, `
		SELECT id FROM deploy_attempts WHERE service_name = ? AND image = ? AND status = ?
		ORDER BY started_at DESC LIMIT 1`, serviceName, image, DeployAttemptStatusSucceeded).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: record rollout for %q: %w", serviceName, err)
	}
	return db.supersedeServingBefore(ctx, id)
}

// supersedeServingBefore marks every other serving attempt of the same app
// superseded once id serves, so only the live release claims to serve.
func (db *DB) supersedeServingBefore(ctx context.Context, id string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET rollout_state = ?
		WHERE rollout_state = ? AND id <> ?
		AND service_name = (SELECT service_name FROM deploy_attempts WHERE id = ?)`,
		RolloutStateSuperseded, RolloutStateServing, id, id)
	if err != nil {
		return fmt.Errorf("store: supersede serving attempts before %q: %w", id, err)
	}
	return nil
}

// RecordRolloutFailure flips the newest succeeded attempt that deployed
// image to failed: its container never passed readiness, so the
// "succeeded" recorded optimistically at trigger time no longer holds.
func (db *DB) RecordRolloutFailure(ctx context.Context, serviceName, image, reason string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, rollout_state = ?, error = ?, finished_at = COALESCE(finished_at, ?)
		WHERE id = (
			SELECT id FROM deploy_attempts
			WHERE service_name = ? AND image = ? AND status = ?
			ORDER BY started_at DESC LIMIT 1
		)
	`, DeployAttemptStatusFailed, RolloutStateFailed, reason, time.Now().UTC().Format(time.RFC3339Nano),
		serviceName, image, DeployAttemptStatusSucceeded)
	if err != nil {
		return fmt.Errorf("store: record rollout failure for %q: %w", serviceName, err)
	}
	return nil
}

// RecoverRestartRollout flips the service's newest attempt back to
// succeeded when it is a restart of image that was marked failed but the
// container has since come up ready. Other sources stay failed, and a
// newer attempt of any kind is never touched. Safe to call every pass.
func (db *DB) RecoverRestartRollout(ctx context.Context, serviceName, image string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, rollout_state = ?, error = '', reason = ''
		WHERE id = (
			SELECT id FROM deploy_attempts WHERE service_name = ?
			ORDER BY started_at DESC LIMIT 1
		) AND image = ? AND source = ? AND status = ?
	`, DeployAttemptStatusSucceeded, RolloutStateServing, serviceName, image, DeployAttemptSourceRestart, DeployAttemptStatusFailed)
	if err != nil {
		return fmt.Errorf("store: recover restart rollout for %q: %w", serviceName, err)
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

// SetDeployAttemptReason records why an attempt was held, superseded or
// allowed through a freeze.
func (db *DB) SetDeployAttemptReason(ctx context.Context, id, reason string) error {
	if _, err := db.ExecContext(ctx, `UPDATE deploy_attempts SET reason = ? WHERE id = ?`, reason, id); err != nil {
		return fmt.Errorf("store: set deploy attempt %q reason: %w", id, err)
	}
	return nil
}
