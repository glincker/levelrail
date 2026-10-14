package secretscan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrRunNotFound means no scan was recorded for the deploy attempt.
var ErrRunNotFound = errors.New("secretscan: no scan recorded for this deploy")

// SQLStore implements Store over the control plane database.
type SQLStore struct{ db *store.DB }

// NewSQLStore wraps db.
func NewSQLStore(db *store.DB) *SQLStore { return &SQLStore{db: db} }

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseStamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetPolicy returns the stored policy, or warn when none is set.
func (s *SQLStore) GetPolicy(ctx context.Context, app string) (Policy, error) {
	var p string
	err := s.db.QueryRowContext(ctx, `SELECT policy FROM app_secret_scan_settings WHERE app_name = ?`, app).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return PolicyWarn, nil
	}
	if err != nil {
		return "", fmt.Errorf("store: get secret scan policy for %q: %w", app, err)
	}
	if pol, perr := ParsePolicy(p); perr == nil {
		return pol, nil
	}
	return PolicyWarn, nil
}

// SetPolicy upserts the policy.
func (s *SQLStore) SetPolicy(ctx context.Context, app string, p Policy) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_secret_scan_settings (app_name, policy, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET policy = excluded.policy, updated_at = excluded.updated_at`, app, string(p), stamp(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set secret scan policy for %q: %w", app, err)
	}
	return nil
}

// ListAllow returns the app's allow-list, oldest first.
func (s *SQLStore) ListAllow(ctx context.Context, app string) ([]AllowEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fingerprint, reason, actor, created_at FROM secret_scan_allowlist WHERE app_name = ? ORDER BY created_at, fingerprint`, app)
	if err != nil {
		return nil, fmt.Errorf("store: list secret scan allow-list for %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	out := []AllowEntry{}
	for rows.Next() {
		e := AllowEntry{App: app}
		var at string
		if err := rows.Scan(&e.Fingerprint, &e.Reason, &e.Actor, &at); err != nil {
			return nil, fmt.Errorf("store: scan secret scan allow-list row: %w", err)
		}
		e.CreatedAt = parseStamp(at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddAllow inserts an entry, keeping the first one's actor and reason on a repeat.
func (s *SQLStore) AddAllow(ctx context.Context, e AllowEntry) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO secret_scan_allowlist (app_name, fingerprint, reason, actor, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(app_name, fingerprint) DO NOTHING`, e.App, e.Fingerprint, e.Reason, e.Actor, stamp(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: add secret scan allow entry for %q: %w", e.App, err)
	}
	return nil
}

// RemoveAllow deletes an entry and reports whether it existed.
func (s *SQLStore) RemoveAllow(ctx context.Context, app, fingerprint string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM secret_scan_allowlist WHERE app_name = ? AND fingerprint = ?`, app, fingerprint)
	if err != nil {
		return false, fmt.Errorf("store: remove secret scan allow entry for %q: %w", app, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SaveRun upserts a run record.
func (s *SQLStore) SaveRun(ctx context.Context, r Run) error {
	findings, err := json.Marshal(r.Findings)
	if err != nil {
		return fmt.Errorf("store: marshal secret scan findings: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO secret_scan_runs (attempt_id, app_name, policy, action, reason, status, error, files_scanned, truncated, findings, scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(attempt_id) DO UPDATE SET policy = excluded.policy, action = excluded.action, reason = excluded.reason, status = excluded.status,
			error = excluded.error, files_scanned = excluded.files_scanned, truncated = excluded.truncated, findings = excluded.findings, scanned_at = excluded.scanned_at`,
		r.AttemptID, r.App, string(r.Policy), r.Action, r.Reason, r.Status, r.Error, r.FilesScanned, r.Truncated, string(findings), stamp(r.ScannedAt))
	if err != nil {
		return fmt.Errorf("store: save secret scan run for %q: %w", r.AttemptID, err)
	}
	return nil
}

// GetRun returns the run of a deploy attempt.
func (s *SQLStore) GetRun(ctx context.Context, attemptID string) (Run, error) {
	var r Run
	var policy, findings, at string
	err := s.db.QueryRowContext(ctx, `SELECT attempt_id, app_name, policy, action, reason, status, error, files_scanned, truncated, findings, scanned_at
		FROM secret_scan_runs WHERE attempt_id = ?`, attemptID).
		Scan(&r.AttemptID, &r.App, &policy, &r.Action, &r.Reason, &r.Status, &r.Error, &r.FilesScanned, &r.Truncated, &findings, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("store: get secret scan run for %q: %w", attemptID, err)
	}
	r.Policy, r.ScannedAt = Policy(policy), parseStamp(at)
	if err := json.Unmarshal([]byte(findings), &r.Findings); err != nil {
		return Run{}, fmt.Errorf("store: decode secret scan findings for %q: %w", attemptID, err)
	}
	return r, nil
}

// DeleteApp removes everything stored for app.
func (s *SQLStore) DeleteApp(ctx context.Context, app string) error {
	for _, q := range []string{
		`DELETE FROM app_secret_scan_settings WHERE app_name = ?`,
		`DELETE FROM secret_scan_allowlist WHERE app_name = ?`,
		`DELETE FROM secret_scan_runs WHERE app_name = ?`,
	} {
		if _, err := s.db.ExecContext(ctx, q, app); err != nil {
			return fmt.Errorf("store: delete secret scan data for %q: %w", app, err)
		}
	}
	return nil
}
