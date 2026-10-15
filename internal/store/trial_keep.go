package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrServiceNotTrial means a keep was asked for a service that was never a
// trial deployment.
var ErrServiceNotTrial = errors.New("store: service is not a trial deployment")

// TrialKeep is the outcome of KeepTrialApp.
type TrialKeep struct {
	// Services is every service of the app that stopped being a trial now;
	// empty when the app had already been kept.
	Services    []string
	KeptAt      time.Time
	AlreadyKept bool
}

// KeepTrialApp makes name's app permanent: every trial service sharing its
// app stops being flagged as throwaway. Keeping an app that was already
// kept reports AlreadyKept; one that never was a trial is ErrServiceNotTrial.
func (db *DB) KeepTrialApp(ctx context.Context, name string) (TrialKeep, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return TrialKeep{}, fmt.Errorf("store: keep trial %q: begin: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var (
		isTrial bool
		keptAt  string
		appID   sql.NullString
	)
	err = tx.QueryRowContext(ctx, `SELECT is_trial, trial_kept_at, app_id FROM desired_services WHERE name = ?`, name).Scan(&isTrial, &keptAt, &appID)
	if errors.Is(err, sql.ErrNoRows) {
		return TrialKeep{}, ErrServiceNotFound
	}
	if err != nil {
		return TrialKeep{}, fmt.Errorf("store: keep trial %q: load: %w", name, err)
	}
	if !isTrial {
		if keptAt == "" {
			return TrialKeep{}, ErrServiceNotTrial
		}
		at, _ := time.Parse(time.RFC3339Nano, keptAt)
		return TrialKeep{KeptAt: at, AlreadyKept: true}, nil
	}

	now := time.Now().UTC()
	const scope = `is_trial = 1 AND (name = ? OR (? != '' AND app_id = ?))`
	rows, err := tx.QueryContext(ctx, `SELECT name FROM desired_services WHERE `+scope+` ORDER BY name`, name, appID.String, appID.String)
	if err != nil {
		return TrialKeep{}, fmt.Errorf("store: keep trial %q: list: %w", name, err)
	}
	out := TrialKeep{KeptAt: now}
	for rows.Next() {
		var svc string
		if err := rows.Scan(&svc); err != nil {
			_ = rows.Close()
			return TrialKeep{}, fmt.Errorf("store: keep trial %q: scan: %w", name, err)
		}
		out.Services = append(out.Services, svc)
	}
	if err := rows.Close(); err != nil {
		return TrialKeep{}, fmt.Errorf("store: keep trial %q: close: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE desired_services SET is_trial = 0, trial_kept_at = ? WHERE `+scope,
		now.Format(time.RFC3339Nano), name, appID.String, appID.String); err != nil {
		return TrialKeep{}, fmt.Errorf("store: keep trial %q: update: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return TrialKeep{}, fmt.Errorf("store: keep trial %q: commit: %w", name, err)
	}
	return out, nil
}
