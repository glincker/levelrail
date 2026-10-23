package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func scanSpendCap(sc interface{ Scan(...any) error }) (spendcap.Cap, error) {
	var c spendcap.Cap
	var thresholds string
	if err := sc.Scan(&c.App, &c.MonthlyEgressBytes, &c.MonthlyCostCeiling, &thresholds, &c.Action, &c.ThrottleRPS, &c.GuardMode); err != nil {
		return c, err
	}
	if err := json.Unmarshal([]byte(thresholds), &c.Thresholds); err != nil {
		return c, fmt.Errorf("decode thresholds: %w", err)
	}
	return c, nil
}

const spendCapColumns = `app_name, monthly_egress_bytes, monthly_cost_ceiling, thresholds, action, throttle_rps, guard_mode`

// ListSpendCaps returns every app spend cap.
func (db *DB) ListSpendCaps(ctx context.Context) ([]spendcap.Cap, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+spendCapColumns+` FROM app_spend_caps ORDER BY app_name`)
	if err != nil {
		return nil, fmt.Errorf("store: list spend caps: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []spendcap.Cap
	for rows.Next() {
		c, err := scanSpendCap(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan spend cap: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetSpendCap returns one app's cap, or spendcap.ErrNotFound.
func (db *DB) GetSpendCap(ctx context.Context, app string) (spendcap.Cap, error) {
	c, err := scanSpendCap(db.QueryRowContext(ctx, `SELECT `+spendCapColumns+` FROM app_spend_caps WHERE app_name = ?`, app))
	if errors.Is(err, sql.ErrNoRows) {
		return c, spendcap.ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("store: get spend cap %q: %w", app, err)
	}
	return c, nil
}

func insertSpendCapEvent(ctx context.Context, tx *sql.Tx, ev spendcap.Event) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO app_spend_cap_events (app_name, period, kind, detail, pct, at) VALUES (?, ?, ?, ?, ?, ?)`,
		ev.App, ev.Period, ev.Kind, ev.Detail, ev.Pct, ev.At.UTC().Format(time.RFC3339))
	return err
}

// SaveSpendCap upserts an app's cap and records ev in its history.
func (db *DB) SaveSpendCap(ctx context.Context, c spendcap.Cap, ev spendcap.Event, now time.Time) error {
	thresholds, err := json.Marshal(c.Thresholds)
	if err != nil {
		return fmt.Errorf("store: encode spend cap thresholds: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save spend cap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO app_spend_caps (app_name, monthly_egress_bytes, monthly_cost_ceiling, thresholds, action, throttle_rps, guard_mode, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET monthly_egress_bytes = excluded.monthly_egress_bytes,
			monthly_cost_ceiling = excluded.monthly_cost_ceiling, thresholds = excluded.thresholds,
			action = excluded.action, throttle_rps = excluded.throttle_rps, guard_mode = excluded.guard_mode, updated_at = excluded.updated_at`,
		c.App, c.MonthlyEgressBytes, c.MonthlyCostCeiling, string(thresholds), c.Action, c.ThrottleRPS, c.GuardMode, now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save spend cap %q: %w", c.App, err)
	}
	if err := insertSpendCapEvent(ctx, tx, ev); err != nil {
		return fmt.Errorf("store: record spend cap event %q: %w", c.App, err)
	}
	return tx.Commit()
}

// DeleteSpendCap removes an app's cap and state, keeping ev as history.
func (db *DB) DeleteSpendCap(ctx context.Context, app string, ev spendcap.Event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete spend cap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM app_spend_caps WHERE app_name = ?`, app)
	if err != nil {
		return fmt.Errorf("store: delete spend cap %q: %w", app, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return spendcap.ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_spend_cap_state WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete spend cap state %q: %w", app, err)
	}
	if err := insertSpendCapEvent(ctx, tx, ev); err != nil {
		return fmt.Errorf("store: record spend cap event %q: %w", app, err)
	}
	return tx.Commit()
}

// GetSpendCapState returns the controller's last decision for an app.
func (db *DB) GetSpendCapState(ctx context.Context, app string) (spendcap.State, bool, error) {
	var st spendcap.State
	var at string
	err := db.QueryRowContext(ctx, `SELECT app_name, period, enforced, nudged, highest, pct, egress_bytes, cost, updated_at FROM app_spend_cap_state WHERE app_name = ?`, app).
		Scan(&st.App, &st.Period, &st.Enforced, &st.Nudged, &st.Highest, &st.Pct, &st.EgressBytes, &st.Cost, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return st, false, nil
	}
	if err != nil {
		return st, false, fmt.Errorf("store: get spend cap state %q: %w", app, err)
	}
	st.UpdatedAt, _ = time.Parse(time.RFC3339, at)
	return st, true, nil
}

// SaveSpendCapState upserts the state and appends events in one transaction.
func (db *DB) SaveSpendCapState(ctx context.Context, st spendcap.State, events []spendcap.Event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save spend cap state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO app_spend_cap_state (app_name, period, enforced, nudged, highest, pct, egress_bytes, cost, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(app_name) DO UPDATE SET period = excluded.period, enforced = excluded.enforced, nudged = excluded.nudged,
			highest = excluded.highest, pct = excluded.pct, egress_bytes = excluded.egress_bytes, cost = excluded.cost, updated_at = excluded.updated_at`,
		st.App, st.Period, st.Enforced, st.Nudged, st.Highest, st.Pct, st.EgressBytes, st.Cost, st.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save spend cap state %q: %w", st.App, err)
	}
	for _, ev := range events {
		if err := insertSpendCapEvent(ctx, tx, ev); err != nil {
			return fmt.Errorf("store: record spend cap event %q: %w", st.App, err)
		}
	}
	return tx.Commit()
}

// ListSpendCapEvents returns an app's history, newest first.
func (db *DB) ListSpendCapEvents(ctx context.Context, app string, limit int) ([]spendcap.Event, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, app_name, period, kind, detail, pct, at FROM app_spend_cap_events WHERE app_name = ? ORDER BY id DESC LIMIT ?`, app, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list spend cap events %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	var out []spendcap.Event
	for rows.Next() {
		var ev spendcap.Event
		var at string
		if err := rows.Scan(&ev.ID, &ev.App, &ev.Period, &ev.Kind, &ev.Detail, &ev.Pct, &at); err != nil {
			return nil, fmt.Errorf("store: scan spend cap event: %w", err)
		}
		ev.At, _ = time.Parse(time.RFC3339, at)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// ListSpendCapEnforcements returns the actions ingress must apply for
// period. A state from an earlier month enforces nothing, so a limit lapses
// at rollover even if the controller is not running.
func (db *DB) ListSpendCapEnforcements(ctx context.Context, period string) ([]spendcap.Enforcement, error) {
	caps, err := db.listAppCapEnforcements(ctx, period)
	if err != nil {
		return nil, err
	}
	budgets, err := db.listCostBudgetEnforcements(ctx, period)
	if err != nil {
		return nil, err
	}
	if len(budgets) == 0 {
		return caps, nil
	}
	return mergeEnforcements(caps, budgets), nil
}

func (db *DB) listAppCapEnforcements(ctx context.Context, period string) ([]spendcap.Enforcement, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.app_name, s.enforced, c.throttle_rps FROM app_spend_cap_state s
		JOIN app_spend_caps c ON c.app_name = s.app_name
		WHERE s.period = ? AND s.enforced != '' ORDER BY s.app_name`, period)
	if err != nil {
		return nil, fmt.Errorf("store: list spend cap enforcements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []spendcap.Enforcement
	for rows.Next() {
		var e spendcap.Enforcement
		if err := rows.Scan(&e.App, &e.Action, &e.ThrottleRPS); err != nil {
			return nil, fmt.Errorf("store: scan spend cap enforcement: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SaveSpendOverride records a root override and its history event together.
func (db *DB) SaveSpendOverride(ctx context.Context, o spendcap.Override, ev spendcap.Event) (spendcap.Override, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return o, fmt.Errorf("store: save spend override: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO spend_guard_overrides (app_name, period, reason, actor, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		o.App, o.Period, o.Reason, o.Actor, o.CreatedAt.UTC().Format(time.RFC3339), o.ExpiresAt.UTC().Format(time.RFC3339))
	if err != nil {
		return o, fmt.Errorf("store: save spend override %q: %w", o.App, err)
	}
	o.ID, _ = res.LastInsertId()
	if err := insertSpendCapEvent(ctx, tx, ev); err != nil {
		return o, fmt.Errorf("store: record spend override event %q: %w", o.App, err)
	}
	return o, tx.Commit()
}

// ActiveSpendOverride returns the latest unexpired override for an app.
func (db *DB) ActiveSpendOverride(ctx context.Context, app string, now time.Time) (spendcap.Override, bool, error) {
	var o spendcap.Override
	var created, expires string
	err := db.QueryRowContext(ctx, `SELECT id, app_name, period, reason, actor, created_at, expires_at FROM spend_guard_overrides
		WHERE app_name = ? AND expires_at > ? ORDER BY expires_at DESC LIMIT 1`, app, now.UTC().Format(time.RFC3339)).
		Scan(&o.ID, &o.App, &o.Period, &o.Reason, &o.Actor, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return o, false, nil
	}
	if err != nil {
		return o, false, fmt.Errorf("store: active spend override %q: %w", app, err)
	}
	o.CreatedAt, _ = time.Parse(time.RFC3339, created)
	o.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
	return o, true, nil
}
