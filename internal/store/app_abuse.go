package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AppAbusePolicy is one row of app_ingress_abuse (migrations/2930): the JSON
// spec of an app's abuse policy and the status condition last reported for it.
// internal/api validates Spec with internal/abuse before saving.
type AppAbusePolicy struct {
	App       string
	Spec      []byte
	UpdatedAt string
	UpdatedBy string

	ConditionStatus  string
	ConditionReason  string
	ConditionMessage string
	ConditionAt      string
}

const appAbuseColumns = `app_name, spec, updated_at, updated_by, condition_status, condition_reason, condition_message, condition_at`

func scanAppAbuse(sc interface{ Scan(...any) error }) (AppAbusePolicy, error) {
	var p AppAbusePolicy
	var spec string
	if err := sc.Scan(&p.App, &spec, &p.UpdatedAt, &p.UpdatedBy, &p.ConditionStatus, &p.ConditionReason, &p.ConditionMessage, &p.ConditionAt); err != nil {
		return AppAbusePolicy{}, err
	}
	p.Spec = []byte(spec)
	return p, nil
}

// GetAppAbusePolicy returns app's policy; found is false when none is set.
func (db *DB) GetAppAbusePolicy(ctx context.Context, app string) (AppAbusePolicy, bool, error) {
	p, err := scanAppAbuse(db.QueryRowContext(ctx, `SELECT `+appAbuseColumns+` FROM app_ingress_abuse WHERE app_name = ?`, app))
	if errors.Is(err, sql.ErrNoRows) {
		return AppAbusePolicy{}, false, nil
	}
	if err != nil {
		return AppAbusePolicy{}, false, fmt.Errorf("store: get app abuse policy %s: %w", app, err)
	}
	return p, true, nil
}

// ListAppAbusePolicies returns every app's policy, read by the ingress
// reconciler on each pass.
func (db *DB) ListAppAbusePolicies(ctx context.Context) ([]AppAbusePolicy, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+appAbuseColumns+` FROM app_ingress_abuse ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list app abuse policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AppAbusePolicy
	for rows.Next() {
		p, err := scanAppAbuse(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan app abuse policy: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app abuse policies: %w", err)
	}
	return out, nil
}

// SetAppAbusePolicy upserts app's spec and clears the stored condition, which
// the next reconcile pass reports afresh.
func (db *DB) SetAppAbusePolicy(ctx context.Context, app string, spec []byte, actor string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_ingress_abuse (app_name, spec, updated_at, updated_by) VALUES (?, ?, ?, ?)
		ON CONFLICT (app_name) DO UPDATE SET spec = excluded.spec, updated_at = excluded.updated_at, updated_by = excluded.updated_by,
			condition_status = '', condition_reason = '', condition_message = '', condition_at = ''
	`, app, string(spec), time.Now().UTC().Format(time.RFC3339), actor)
	if err != nil {
		return fmt.Errorf("store: set app abuse policy %s: %w", app, err)
	}
	return nil
}

// DeleteAppAbusePolicy removes app's policy, returning it to the default (off). Idempotent.
func (db *DB) DeleteAppAbusePolicy(ctx context.Context, app string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM app_ingress_abuse WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete app abuse policy %s: %w", app, err)
	}
	return nil
}

// SetAppAbuseCondition records the status condition for app, touching the row
// only when it changed so an idle reconcile loop does not write.
func (db *DB) SetAppAbuseCondition(ctx context.Context, app, status, reason, message string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE app_ingress_abuse SET condition_status = ?, condition_reason = ?, condition_message = ?, condition_at = ?
		WHERE app_name = ? AND (condition_status != ? OR condition_reason != ? OR condition_message != ?)
	`, status, reason, message, time.Now().UTC().Format(time.RFC3339), app, status, reason, message)
	if err != nil {
		return fmt.Errorf("store: set app abuse condition %s: %w", app, err)
	}
	return nil
}
