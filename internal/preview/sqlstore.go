package preview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const recordColumns = `deployment_id, app_name, path, bytes, width, height, status, reason, detail, http_status, captured_at, last_viewed_at`

// SQLStore implements Store over the control plane database.
type SQLStore struct{ db *store.DB }

// NewSQLStore wraps db.
func NewSQLStore(db *store.DB) *SQLStore { return &SQLStore{db: db} }

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetPreviewSettings returns the stored settings, or the zero value when none exist.
func (s *SQLStore) GetPreviewSettings(ctx context.Context, app string) (AppSettings, error) {
	var out AppSettings
	err := s.db.QueryRowContext(ctx, `SELECT enabled, path, wait_ms FROM deploy_preview_settings WHERE app_name = ?`, app).Scan(&out.Enabled, &out.Path, &out.WaitMS)
	if errors.Is(err, sql.ErrNoRows) {
		return AppSettings{App: app, Path: DefaultPath}, nil
	}
	if err != nil {
		return AppSettings{}, fmt.Errorf("store: get preview settings for %q: %w", app, err)
	}
	out.App = app
	return out, nil
}

// SavePreviewSettings upserts settings.
func (s *SQLStore) SavePreviewSettings(ctx context.Context, a AppSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deploy_preview_settings (app_name, enabled, path, wait_ms, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET enabled = excluded.enabled, path = excluded.path, wait_ms = excluded.wait_ms, updated_at = excluded.updated_at
	`, a.App, a.Enabled, a.Path, a.WaitMS, fmtTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: save preview settings for %q: %w", a.App, err)
	}
	return nil
}

// DeletePreviewSettings removes an app's settings, returning it to the default off state.
func (s *SQLStore) DeletePreviewSettings(ctx context.Context, app string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM deploy_preview_settings WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete preview settings for %q: %w", app, err)
	}
	return nil
}

// LatestServingDeployment is LatestSucceededDeployment restricted to attempts
// the controller has observed serving, so a preview is never filed under a
// release that has not replaced the old one yet.
func (s *SQLStore) LatestServingDeployment(ctx context.Context, service, image string) (string, error) {
	q := `SELECT id FROM deploy_attempts WHERE service_name = ? AND status = ? AND rollout_state = ?`
	args := []any{service, store.DeployAttemptStatusSucceeded, store.RolloutStateServing}
	if image != "" {
		q += ` AND image = ?`
		args = append(args, image)
	}
	var id string
	err := s.db.QueryRowContext(ctx, q+` ORDER BY started_at DESC LIMIT 1`, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: latest serving deployment of %q: %w", service, err)
	}
	return id, nil
}

// UpsertPreviewRecord inserts or replaces a record, keeping its view time.
func (s *SQLStore) UpsertPreviewRecord(ctx context.Context, r Record) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deploy_previews (`+recordColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(deployment_id) DO UPDATE SET app_name = excluded.app_name, path = excluded.path, bytes = excluded.bytes,
			width = excluded.width, height = excluded.height, status = excluded.status, reason = excluded.reason,
			detail = excluded.detail, http_status = excluded.http_status, captured_at = excluded.captured_at
	`, r.DeploymentID, r.App, r.Path, r.Bytes, r.Width, r.Height, r.Status, r.Reason, r.Detail, r.HTTPStatus, fmtTime(r.CapturedAt), fmtTime(r.LastViewedAt))
	if err != nil {
		return fmt.Errorf("store: upsert preview record %q: %w", r.DeploymentID, err)
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func scanRecord(sc rowScanner) (Record, error) {
	var r Record
	var captured, viewed string
	if err := sc.Scan(&r.DeploymentID, &r.App, &r.Path, &r.Bytes, &r.Width, &r.Height, &r.Status, &r.Reason, &r.Detail, &r.HTTPStatus, &captured, &viewed); err != nil {
		return Record{}, err
	}
	r.CapturedAt, r.LastViewedAt = parseTime(captured), parseTime(viewed)
	return r, nil
}

// GetPreviewRecord returns one record, or nil when absent.
func (s *SQLStore) GetPreviewRecord(ctx context.Context, deploymentID string) (*Record, error) {
	r, err := scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM deploy_previews WHERE deployment_id = ?`, deploymentID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get preview record %q: %w", deploymentID, err)
	}
	return &r, nil
}

func (s *SQLStore) queryRecords(ctx context.Context, where string, args ...any) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordColumns+` FROM deploy_previews `+where+` ORDER BY captured_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query preview records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan preview record: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate preview records: %w", err)
	}
	return out, nil
}

// ListPreviewRecords lists one app's records, newest first.
func (s *SQLStore) ListPreviewRecords(ctx context.Context, app string) ([]Record, error) {
	return s.queryRecords(ctx, `WHERE app_name = ?`, app)
}

// ListAllPreviewRecords lists every record, newest first.
func (s *SQLStore) ListAllPreviewRecords(ctx context.Context) ([]Record, error) {
	return s.queryRecords(ctx, ``)
}

// DeletePreviewRecords removes records by deployment ID.
func (s *SQLStore) DeletePreviewRecords(ctx context.Context, ids []string) error {
	const chunk = 400
	for start := 0; start < len(ids); start += chunk {
		part := ids[start:min(start+chunk, len(ids))]
		args := make([]any, len(part))
		for i, id := range part {
			args[i] = id
		}
		q := `DELETE FROM deploy_previews WHERE deployment_id IN (?` + strings.Repeat(",?", len(part)-1) + `)`
		if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("store: delete preview records: %w", err)
		}
	}
	return nil
}

// TouchPreviewViewed stamps the last time a preview image was served.
func (s *SQLStore) TouchPreviewViewed(ctx context.Context, deploymentID string, at time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE deploy_previews SET last_viewed_at = ? WHERE deployment_id = ?`, fmtTime(at), deploymentID); err != nil {
		return fmt.Errorf("store: touch preview %q: %w", deploymentID, err)
	}
	return nil
}

// GetPreviewState reads one state value, "" when unset.
func (s *SQLStore) GetPreviewState(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM deploy_preview_state WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get preview state %q: %w", key, err)
	}
	return v, nil
}

// SetPreviewState writes one state value.
func (s *SQLStore) SetPreviewState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO deploy_preview_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("store: set preview state %q: %w", key, err)
	}
	return nil
}

// LatestSucceededDeployment returns the newest succeeded deploy attempt of
// service, restricted to image when non-empty. "" means none.
func (s *SQLStore) LatestSucceededDeployment(ctx context.Context, service, image string) (string, error) {
	q := `SELECT id FROM deploy_attempts WHERE service_name = ? AND status = ?`
	args := []any{service, store.DeployAttemptStatusSucceeded}
	if image != "" {
		q += ` AND image = ?`
		args = append(args, image)
	}
	var id string
	err := s.db.QueryRowContext(ctx, q+` ORDER BY started_at DESC LIMIT 1`, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: latest succeeded deployment of %q: %w", service, err)
	}
	return id, nil
}
