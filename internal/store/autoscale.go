package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/autoscale"
)

const (
	autoscaleColumns = `app_name, min_replicas, max_replicas, metric, target, scale_up_tolerance_pct, scale_down_tolerance_pct,
		scale_up_cooldown_secs, scale_down_cooldown_secs, stabilization_secs, max_step_up, max_step_down`
	autoscaleEventsKept = 500
	autoscaleActor      = "autoscaler"
)

func scanAutoscalePolicy(sc interface{ Scan(...any) error }) (autoscale.Policy, error) {
	var p autoscale.Policy
	err := sc.Scan(&p.App, &p.MinReplicas, &p.MaxReplicas, &p.Metric, &p.Target, &p.ScaleUpTolerancePct, &p.ScaleDownTolerancePct,
		&p.ScaleUpCooldownSecs, &p.ScaleDownCooldownSecs, &p.StabilizationSecs, &p.MaxStepUp, &p.MaxStepDown)
	return p, err
}

func formatAutoscaleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseAutoscaleTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// ListAutoscalePolicies returns every app autoscale policy.
func (db *DB) ListAutoscalePolicies(ctx context.Context) ([]autoscale.Policy, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+autoscaleColumns+` FROM app_autoscale ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list autoscale policies: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []autoscale.Policy
	for rows.Next() {
		p, err := scanAutoscalePolicy(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan autoscale policy: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetAutoscalePolicy returns one app's policy, or autoscale.ErrNotFound.
func (db *DB) GetAutoscalePolicy(ctx context.Context, app string) (autoscale.Policy, error) {
	p, err := scanAutoscalePolicy(db.QueryRowContext(ctx, `SELECT `+autoscaleColumns+` FROM app_autoscale WHERE app_name = ?`, app))
	if errors.Is(err, sql.ErrNoRows) {
		return p, autoscale.ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("store: get autoscale policy %q: %w", app, err)
	}
	return p, nil
}

func insertAutoscaleEvent(ctx context.Context, tx *sql.Tx, ev autoscale.Event) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_autoscale_events (app_name, kind, current, desired, value, reason, blocked_by, detail, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.App, ev.Kind, ev.Current, ev.Desired, ev.Value, ev.Reason, ev.BlockedBy, ev.Detail, ev.At.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM app_autoscale_events WHERE app_name = ? AND id NOT IN
		(SELECT id FROM app_autoscale_events WHERE app_name = ? ORDER BY id DESC LIMIT ?)`, ev.App, ev.App, autoscaleEventsKept)
	return err
}

// SaveAutoscalePolicy upserts an app's policy and records ev in its history.
func (db *DB) SaveAutoscalePolicy(ctx context.Context, p autoscale.Policy, ev autoscale.Event, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save autoscale policy: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO app_autoscale (`+autoscaleColumns+`, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET min_replicas = excluded.min_replicas, max_replicas = excluded.max_replicas,
			metric = excluded.metric, target = excluded.target, scale_up_tolerance_pct = excluded.scale_up_tolerance_pct,
			scale_down_tolerance_pct = excluded.scale_down_tolerance_pct, scale_up_cooldown_secs = excluded.scale_up_cooldown_secs,
			scale_down_cooldown_secs = excluded.scale_down_cooldown_secs, stabilization_secs = excluded.stabilization_secs,
			max_step_up = excluded.max_step_up, max_step_down = excluded.max_step_down, updated_at = excluded.updated_at`,
		p.App, p.MinReplicas, p.MaxReplicas, p.Metric, p.Target, p.ScaleUpTolerancePct, p.ScaleDownTolerancePct,
		p.ScaleUpCooldownSecs, p.ScaleDownCooldownSecs, p.StabilizationSecs, p.MaxStepUp, p.MaxStepDown, now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save autoscale policy %q: %w", p.App, err)
	}
	if err := insertAutoscaleEvent(ctx, tx, ev); err != nil {
		return fmt.Errorf("store: record autoscale event %q: %w", p.App, err)
	}
	return tx.Commit()
}

// DeleteAutoscalePolicy removes an app's policy and state, keeping ev as history.
func (db *DB) DeleteAutoscalePolicy(ctx context.Context, app string, ev autoscale.Event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete autoscale policy: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM app_autoscale WHERE app_name = ?`, app)
	if err != nil {
		return fmt.Errorf("store: delete autoscale policy %q: %w", app, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return autoscale.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_autoscale_state WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete autoscale state %q: %w", app, err)
	}
	if err := insertAutoscaleEvent(ctx, tx, ev); err != nil {
		return fmt.Errorf("store: record autoscale event %q: %w", app, err)
	}
	return tx.Commit()
}

// GetAutoscaleState returns the controller's last decision for an app.
func (db *DB) GetAutoscaleState(ctx context.Context, app string) (autoscale.State, bool, error) {
	var st autoscale.State
	var known int
	var up, down, since, at string
	err := db.QueryRowContext(ctx, `SELECT app_name, current, desired, value, value_known, reason, blocked_by, message,
		last_scale_up, last_scale_down, down_since, down_floor, updated_at FROM app_autoscale_state WHERE app_name = ?`, app).
		Scan(&st.App, &st.Current, &st.Desired, &st.Value, &known, &st.Reason, &st.BlockedBy, &st.Message, &up, &down, &since, &st.DownFloor, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return st, false, nil
	}
	if err != nil {
		return st, false, fmt.Errorf("store: get autoscale state %q: %w", app, err)
	}
	st.ValueKnown = known != 0
	st.LastScaleUp, st.LastScaleDown, st.DownSince, st.UpdatedAt = parseAutoscaleTime(up), parseAutoscaleTime(down), parseAutoscaleTime(since), parseAutoscaleTime(at)
	return st, true, nil
}

// SaveAutoscaleState upserts the state and appends events in one transaction.
func (db *DB) SaveAutoscaleState(ctx context.Context, st autoscale.State, events []autoscale.Event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save autoscale state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	known := 0
	if st.ValueKnown {
		known = 1
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO app_autoscale_state (app_name, current, desired, value, value_known, reason, blocked_by, message,
			last_scale_up, last_scale_down, down_since, down_floor, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET current = excluded.current, desired = excluded.desired, value = excluded.value,
			value_known = excluded.value_known, reason = excluded.reason, blocked_by = excluded.blocked_by, message = excluded.message,
			last_scale_up = excluded.last_scale_up, last_scale_down = excluded.last_scale_down, down_since = excluded.down_since,
			down_floor = excluded.down_floor, updated_at = excluded.updated_at`,
		st.App, st.Current, st.Desired, st.Value, known, st.Reason, st.BlockedBy, st.Message,
		formatAutoscaleTime(st.LastScaleUp), formatAutoscaleTime(st.LastScaleDown), formatAutoscaleTime(st.DownSince), st.DownFloor, formatAutoscaleTime(st.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save autoscale state %q: %w", st.App, err)
	}
	for _, ev := range events {
		if err := insertAutoscaleEvent(ctx, tx, ev); err != nil {
			return fmt.Errorf("store: record autoscale event %q: %w", st.App, err)
		}
	}
	return tx.Commit()
}

// ListAutoscaleEvents returns an app's history, newest first.
func (db *DB) ListAutoscaleEvents(ctx context.Context, app string, limit int) ([]autoscale.Event, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, app_name, kind, current, desired, value, reason, blocked_by, detail, at
		FROM app_autoscale_events WHERE app_name = ? ORDER BY id DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list autoscale events %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	var out []autoscale.Event
	for rows.Next() {
		var ev autoscale.Event
		var at string
		if err := rows.Scan(&ev.ID, &ev.App, &ev.Kind, &ev.Current, &ev.Desired, &ev.Value, &ev.Reason, &ev.BlockedBy, &ev.Detail, &at); err != nil {
			return nil, fmt.Errorf("store: scan autoscale event: %w", err)
		}
		ev.At = parseAutoscaleTime(at)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// AutoscaleReplicas returns an app's desired replica count and whether it is suspended.
func (db *DB) AutoscaleReplicas(ctx context.Context, app string) (int, bool, error) {
	var replicas int
	var suspended bool
	err := db.QueryRowContext(ctx, `SELECT replicas, suspended FROM desired_services WHERE name = ?`, app).Scan(&replicas, &suspended)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, autoscale.ErrNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: read replicas for %q: %w", app, err)
	}
	return replicas, suspended, nil
}

// SetAutoscaleReplicas writes an app's desired replica count for the
// autoscaler, the same single-purpose setter shape UpdateServiceSuspended uses.
func (db *DB) SetAutoscaleReplicas(ctx context.Context, app string, replicas int) error {
	var before int
	if err := db.QueryRowContext(ctx, `SELECT replicas FROM desired_services WHERE name = ?`, app).Scan(&before); errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	} else if err != nil {
		return fmt.Errorf("store: read replicas for %q: %w", app, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE desired_services SET replicas = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, replicas, app); err != nil {
		return fmt.Errorf("store: set replicas for %q: %w", app, err)
	}
	// The timeline entry is best effort: the replica write already landed.
	_ = db.AddAppEvent(ctx, AppEvent{
		AppName: app, Kind: AppEventScale, Actor: autoscaleActor,
		Title: fmt.Sprintf("Autoscaler scaled %d to %d replicas", max(before, 1), replicas),
	})
	return db.noteAppChange(ctx, app)
}
