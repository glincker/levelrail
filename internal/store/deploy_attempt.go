package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// DeployAttempt is one row-per-attempt entry in the deploy_attempts
// table: real deploy history, the gap
// docs-local/research/deploy-attempt-id-and-log-persistence.md documents
// against reconcile_status/UpsertConditions only ever keeping the latest
// condition per (controller, type) pair. Minted by all three real
// trigger paths (the plain image-tag deploy, the manual git-source
// build, and the unattended git-push webhook), one row per call.
type DeployAttempt struct {
	// ID is an opaque, mint-time-random identifier from
	// NewDeployAttemptID, not a database-assigned sequence: see that
	// function's own doc comment for why.
	ID          string
	ServiceName string
	// Image is the tag this attempt deploys (or is trying to build and
	// deploy). For the plain image-tag path this is exactly the
	// caller-supplied tag; for the two build-triggering paths it is
	// computed the same way internal/deploy's own deployDockerfile
	// computes it (ImageRepo + ":" + CommitSHA), known before the build
	// even starts, so a failed build's attempt row still shows what tag
	// it was trying to produce.
	Image string
	// CommitSHA is the git commit this attempt built from, empty for the
	// plain image-tag trigger path (DeployAttemptSourceImage), which has
	// no associated commit.
	CommitSHA string
	// Source is one of the DeployAttemptSource* constants.
	Source string
	// Status is one of the DeployAttemptStatus* constants.
	Status    string
	StartedAt time.Time
	// FinishedAt is nil until FinishDeployAttempt is called.
	FinishedAt *time.Time
	// Error is the attempt's failure detail, set only when Status is
	// DeployAttemptStatusFailed. Unlike an HTTP error response (see
	// internal/api/builds.go's handleTriggerBuild doc comment on why
	// that response body stays non-leaky), this is stored for an
	// authenticated operator's own later inspection via
	// GET /api/v1/apps/{name}/deploy-attempts, the same trust boundary
	// GetConditions' Message field already crosses for reconcile
	// failures.
	Error string
}

// Deploy attempt lifecycle. There is no "pending" state distinct from
// "running": every trigger handler saves the row and immediately begins
// work (SaveDesiredService for the plain path, Pipeline.Deploy for the
// two build-triggering paths) in the same call, so there is no
// observable gap between "queued" and "started" worth a separate status
// value.
const (
	DeployAttemptStatusRunning   = "running"
	DeployAttemptStatusSucceeded = "succeeded"
	DeployAttemptStatusFailed    = "failed"
)

// Deploy attempt trigger sources, one per real call site: the
// unattended git-push webhook (internal/webhook), a manual git-source
// build triggered from the dashboard (internal/api/deploy_attempts.go's
// beginBuildDeployAttempt), and a plain image-tag redeploy/rollback with
// no build step (internal/api/deploys.go's recordPlainDeployAttempt).
const (
	DeployAttemptSourceWebhook = "webhook"
	DeployAttemptSourceManual  = "manual"
	DeployAttemptSourceImage   = "image"
)

// deployAttemptIDPrefix mirrors internal/api/tokens.go's "tok_" prefix
// convention, adapted to this table: a short, greppable tag on an
// otherwise-opaque random ID, useful in logs and URLs.
const deployAttemptIDPrefix = "dep_"

// NewDeployAttemptID generates an opaque, URL-safe deploy-attempt
// identifier, minted the same way internal/api/tokens.go's
// randomTokenID mints an API token ID (fixed-length crypto/rand bytes,
// base64 URL encoding, a short prefix), per the design note this
// implements (docs-local/research/deploy-attempt-id-and-log-persistence.md
// section 1, option A). Exported and placed in this package, unlike
// randomTokenID which stays private to internal/api: more than one
// package needs to mint one of these. The plain image-tag and manual
// build triggers both live in internal/api, but the git webhook
// receiver (internal/webhook) is a separate package needing the
// identical scheme, and duplicating the byte-length/encoding choice in
// two places would risk them silently drifting apart.
func NewDeployAttemptID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate deploy attempt id: %w", err)
	}
	return deployAttemptIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveDeployAttempt inserts a new deploy attempt row. Unlike
// SaveDesiredService, this is insert-only, never an upsert: an attempt
// ID is minted fresh by every trigger call (NewDeployAttemptID), so a
// second SaveDeployAttempt for the same ID would only ever indicate a
// caller bug, not a legitimate retry, and is left to fail on the
// primary key constraint rather than silently overwriting history.
func (db *DB) SaveDeployAttempt(ctx context.Context, a DeployAttempt) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_attempts (id, service_name, image, commit_sha, source, status, started_at, finished_at, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, NULL, NULL)
	`, a.ID, a.ServiceName, a.Image, a.CommitSHA, a.Source, a.Status, a.StartedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: save deploy attempt %q: %w", a.ID, err)
	}
	return nil
}

// ErrDeployAttemptNotFound is returned by GetDeployAttempt and
// FinishDeployAttempt when no row has the given ID.
var ErrDeployAttemptNotFound = errors.New("store: deploy attempt not found")

// FinishDeployAttempt marks a deploy attempt's terminal outcome: called
// exactly once per attempt, when the triggering handler's call to
// internal/deploy.Pipeline.Deploy (or, for the plain image-tag path,
// SaveDesiredService) returns. errMsg should be empty for a succeeded
// attempt; status is not validated against a fixed enum here (the three
// call sites already only ever pass one of the two terminal
// DeployAttemptStatus* constants), matching UpsertConditions' own
// "caller controls the vocabulary" convention for status strings
// elsewhere in this package.
func (db *DB) FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error {
	var errVal sql.NullString
	if errMsg != "" {
		errVal = sql.NullString{String: errMsg, Valid: true}
	}
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, finished_at = ?, error = ? WHERE id = ?
	`, status, finishedAt.UTC().Format(time.RFC3339Nano), errVal, id)
	if err != nil {
		return fmt.Errorf("store: finish deploy attempt %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: finish deploy attempt %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrDeployAttemptNotFound
	}
	return nil
}

// GetDeployAttempt returns one deploy attempt by ID, or
// ErrDeployAttemptNotFound if no such row exists. Used by the SSE log
// stream handler (internal/api/deploys.go) to decide whether an
// attempt is still in progress (serve a live tail) or already finished
// (serve a full persisted replay), see that handler's own doc comment.
func (db *DB) GetDeployAttempt(ctx context.Context, id string) (*DeployAttempt, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, service_name, image, commit_sha, source, status, started_at, finished_at, error
		FROM deploy_attempts WHERE id = ?
	`, id)
	a, err := scanDeployAttempt(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeployAttemptNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get deploy attempt %q: %w", id, err)
	}
	return a, nil
}

// ListDeployAttempts returns every attempt for serviceName, newest
// first. A service with no attempts yet returns an empty (nil) slice and
// a nil error, not an error, the same "absence is not an error"
// convention GetConditions/QueryLogs already establish. No pagination:
// per this task's own scope note, an app that accumulates a very large
// number of attempts over its lifetime will make this list grow
// unbounded; that's a known, deliberately deferred follow-up, not solved
// here.
func (db *DB) ListDeployAttempts(ctx context.Context, serviceName string) ([]DeployAttempt, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, image, commit_sha, source, status, started_at, finished_at, error
		FROM deploy_attempts
		WHERE service_name = ?
		ORDER BY started_at DESC
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy attempts for %q: %w", serviceName, err)
	}
	defer func() { _ = rows.Close() }()

	var out []DeployAttempt
	for rows.Next() {
		a, err := scanDeployAttempt(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan deploy attempt row: %w", err)
		}
		out = append(out, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate deploy attempt rows: %w", err)
	}
	return out, nil
}

func scanDeployAttempt(scan func(dest ...any) error) (*DeployAttempt, error) {
	var (
		a                     DeployAttempt
		startedAt             string
		finishedAt, errString sql.NullString
	)
	if err := scan(&a.ID, &a.ServiceName, &a.Image, &a.CommitSHA, &a.Source, &a.Status, &startedAt, &finishedAt, &errString); err != nil {
		return nil, err
	}

	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return nil, fmt.Errorf("parse started_at %q: %w", startedAt, err)
	}
	a.StartedAt = started

	if finishedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, finishedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse finished_at %q: %w", finishedAt.String, err)
		}
		a.FinishedAt = &t
	}
	a.Error = errString.String

	return &a, nil
}
