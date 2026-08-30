package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

// previewEnvironmentIDPrefix mirrors deployAttemptIDPrefix's own
// scheme (deploy_attempt.go): a short, greppable prefix on an otherwise
// opaque random ID.
const previewEnvironmentIDPrefix = "prev_"

// NewPreviewEnvironmentID mints a random preview environment ID, the
// same fixed-length crypto/rand-plus-base64 scheme NewDeployAttemptID
// already establishes for an analogous webhook-minted identifier.
func NewPreviewEnvironmentID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate preview environment id: %w", err)
	}
	return previewEnvironmentIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Preview environment statuses (migrations/0064_preview_environments.sql).
// Deploying is the initial state a webhook-triggered create/update
// writes before the build even starts; Active means the preview is
// reachable (with or without its intended domain, see StatusReason);
// Failed means the deploy itself never produced a running preview.
const (
	PreviewStatusDeploying = "deploying"
	PreviewStatusActive    = "active"
	PreviewStatusFailed    = "failed"
)

// ErrPreviewEnvironmentNotFound is returned by GetPreviewEnvironment and
// GetPreviewEnvironmentByAppAndPR when no row matches.
var ErrPreviewEnvironmentNotFound = errors.New("store: preview environment not found")

// PreviewEnvironment is one pull request's preview deployment
// (migrations/0064_preview_environments.sql): a store.App/DesiredService
// named PreviewAppID, deployed from Branch at HeadSHA, optionally tagged
// with EnvironmentID and reachable at Domain.
type PreviewEnvironment struct {
	ID           string
	AppName      string
	PRNumber     int
	PreviewAppID string
	// EnvironmentID is the shared Preview-tier store.Environment this
	// preview is tagged with, empty when tagging failed or hasn't run
	// yet; see internal/api's ensurePreviewEnvironmentTier.
	EnvironmentID string
	Branch        string
	HeadSHA       string
	// Domain is empty when no base domain is configured, or when a
	// domain collision forced this preview to deploy without one (see
	// StatusReason for that case).
	Domain string
	Status string
	// StatusReason explains a non-obvious Status, e.g. a domain
	// collision that left the preview Active but domain-less, or a
	// build failure's own error message when Status is Failed. Empty for
	// the ordinary success case.
	StatusReason string
	CreatedAt    string
	UpdatedAt    string
}

// SavePreviewEnvironment inserts a new preview environment row.
// Insert-only: a fresh (app_name, pr_number) pair always mints a new ID,
// see UpdatePreviewEnvironment for the redeploy-on-synchronize path.
func (db *DB) SavePreviewEnvironment(ctx context.Context, p PreviewEnvironment) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO preview_environments
			(id, app_name, pr_number, preview_app_id, environment_id, branch, head_sha, domain, status, status_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.AppName, p.PRNumber, p.PreviewAppID,
		sql.NullString{String: p.EnvironmentID, Valid: p.EnvironmentID != ""},
		p.Branch, p.HeadSHA,
		sql.NullString{String: p.Domain, Valid: p.Domain != ""},
		p.Status,
		sql.NullString{String: p.StatusReason, Valid: p.StatusReason != ""},
		p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save preview environment %q: %w", p.ID, err)
	}
	return nil
}

// UpdatePreviewEnvironment replaces an existing preview environment row
// in full, keyed by ID: a synchronize push (new commits on the same PR)
// or a status transition (deploying -> active/failed) both rewrite the
// whole record, the same "no partial update" convention
// UpdateIngressSettings already establishes for a similarly small
// record.
func (db *DB) UpdatePreviewEnvironment(ctx context.Context, p PreviewEnvironment) error {
	res, err := db.ExecContext(ctx, `
		UPDATE preview_environments SET
			branch = ?, head_sha = ?, environment_id = ?, domain = ?, status = ?, status_reason = ?, updated_at = ?
		WHERE id = ?
	`, p.Branch, p.HeadSHA,
		sql.NullString{String: p.EnvironmentID, Valid: p.EnvironmentID != ""},
		sql.NullString{String: p.Domain, Valid: p.Domain != ""},
		p.Status,
		sql.NullString{String: p.StatusReason, Valid: p.StatusReason != ""},
		p.UpdatedAt, p.ID)
	if err != nil {
		return fmt.Errorf("store: update preview environment %q: %w", p.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update preview environment %q: rows affected: %w", p.ID, err)
	}
	if n == 0 {
		return ErrPreviewEnvironmentNotFound
	}
	return nil
}

// GetPreviewEnvironmentByAppAndPR returns the preview environment for
// (appName, prNumber), or ErrPreviewEnvironmentNotFound: the lookup a
// synchronize or closed pull request webhook event uses to find the
// preview it already owns, matching the table's own unique index.
func (db *DB) GetPreviewEnvironmentByAppAndPR(ctx context.Context, appName string, prNumber int) (*PreviewEnvironment, error) {
	row := db.QueryRowContext(ctx, `
		SELECT `+previewEnvironmentColumns+`
		FROM preview_environments WHERE app_name = ? AND pr_number = ?
	`, appName, prNumber)
	p, err := scanPreviewEnvironment(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPreviewEnvironmentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get preview environment for %q pr %d: %w", appName, prNumber, err)
	}
	return p, nil
}

// ListPreviewEnvironmentsByApp returns every preview environment for
// appName, newest first: GET /api/v1/apps/{name}/previews's read path.
func (db *DB) ListPreviewEnvironmentsByApp(ctx context.Context, appName string) ([]PreviewEnvironment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+previewEnvironmentColumns+`
		FROM preview_environments WHERE app_name = ? ORDER BY pr_number DESC
	`, appName)
	if err != nil {
		return nil, fmt.Errorf("store: list preview environments for %q: %w", appName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []PreviewEnvironment
	for rows.Next() {
		p, err := scanPreviewEnvironment(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan preview environment row: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate preview environment rows: %w", err)
	}
	return out, nil
}

// DeletePreviewEnvironment removes a preview environment row once its
// app/services and domain have actually been torn down; returns
// ErrPreviewEnvironmentNotFound if id doesn't exist. A partially-failed
// teardown instead calls UpdatePreviewEnvironment with a Failed status
// and a StatusReason, keeping the row so the manual teardown action has
// something to retry.
func (db *DB) DeletePreviewEnvironment(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM preview_environments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete preview environment %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete preview environment %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrPreviewEnvironmentNotFound
	}
	return nil
}

const previewEnvironmentColumns = "id, app_name, pr_number, preview_app_id, environment_id, branch, head_sha, domain, status, status_reason, created_at, updated_at"

func scanPreviewEnvironment(scan func(dest ...any) error) (*PreviewEnvironment, error) {
	var (
		p                     PreviewEnvironment
		environmentID, domain sql.NullString
		statusReason          sql.NullString
	)
	if err := scan(&p.ID, &p.AppName, &p.PRNumber, &p.PreviewAppID, &environmentID, &p.Branch, &p.HeadSHA, &domain, &p.Status, &statusReason, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.EnvironmentID = environmentID.String
	p.Domain = domain.String
	p.StatusReason = statusReason.String
	return &p, nil
}
