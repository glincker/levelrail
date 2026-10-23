package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/costbudget"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

// ErrCostBudgetNameTaken is returned when another budget already uses a name.
var ErrCostBudgetNameTaken = errors.New("store: a cost budget with that name already exists")

const costBudgetColumns = `id, name, scope, scope_value, monthly, thresholds, action, throttle_rps, guard_mode`

func scanCostBudget(sc interface{ Scan(...any) error }) (costbudget.Budget, error) {
	var b costbudget.Budget
	var thresholds string
	if err := sc.Scan(&b.ID, &b.Name, &b.Scope, &b.ScopeValue, &b.Monthly, &thresholds, &b.Action, &b.ThrottleRPS, &b.GuardMode); err != nil {
		return b, err
	}
	if err := json.Unmarshal([]byte(thresholds), &b.Thresholds); err != nil {
		return b, fmt.Errorf("decode thresholds: %w", err)
	}
	return b, nil
}

// ListCostBudgets returns every named budget.
func (db *DB) ListCostBudgets(ctx context.Context) ([]costbudget.Budget, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+costBudgetColumns+` FROM cost_budgets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list cost budgets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []costbudget.Budget
	for rows.Next() {
		b, err := scanCostBudget(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan cost budget: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetCostBudget returns one budget, or costbudget.ErrNotFound.
func (db *DB) GetCostBudget(ctx context.Context, id string) (costbudget.Budget, error) {
	b, err := scanCostBudget(db.QueryRowContext(ctx, `SELECT `+costBudgetColumns+` FROM cost_budgets WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return b, costbudget.ErrNotFound
	}
	if err != nil {
		return b, fmt.Errorf("store: get cost budget %q: %w", id, err)
	}
	return b, nil
}

// SaveCostBudget inserts a budget, or updates it when ID is set, and returns it.
func (db *DB) SaveCostBudget(ctx context.Context, b costbudget.Budget, now time.Time) (costbudget.Budget, error) {
	ts := now.UTC().Format(time.RFC3339)
	thresholds, err := json.Marshal(b.Thresholds)
	if err != nil {
		return b, fmt.Errorf("store: encode thresholds: %w", err)
	}
	if b.ID == "" {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return b, fmt.Errorf("store: generate cost budget id: %w", err)
		}
		b.ID = "cb_" + hex.EncodeToString(buf[:])
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO cost_budgets (id, name, scope, scope_value, monthly, thresholds, action, throttle_rps, guard_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, scope = excluded.scope, scope_value = excluded.scope_value,
			monthly = excluded.monthly, thresholds = excluded.thresholds, action = excluded.action,
			throttle_rps = excluded.throttle_rps, guard_mode = excluded.guard_mode, updated_at = excluded.updated_at`,
		b.ID, b.Name, b.Scope, b.ScopeValue, b.Monthly, string(thresholds), b.Action, b.ThrottleRPS, b.GuardMode, ts, ts)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: cost_budgets.name") {
			return b, ErrCostBudgetNameTaken
		}
		return b, fmt.Errorf("store: save cost budget: %w", err)
	}
	return b, nil
}

// DeleteCostBudget removes a budget and its state.
func (db *DB) DeleteCostBudget(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM cost_budgets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete cost budget %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return costbudget.ErrNotFound
	}
	return nil
}

// GetCostBudgetState returns the stored state of one budget.
func (db *DB) GetCostBudgetState(ctx context.Context, id string) (costbudget.State, bool, error) {
	var st costbudget.State
	var apps, ts string
	err := db.QueryRowContext(ctx, `SELECT budget_id, period, highest, pct, spent, enforced, enforced_apps, updated_at
		FROM cost_budget_state WHERE budget_id = ?`, id).
		Scan(&st.BudgetID, &st.Period, &st.Highest, &st.Pct, &st.Spent, &st.Enforced, &apps, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return st, false, nil
	}
	if err != nil {
		return st, false, fmt.Errorf("store: get cost budget state %q: %w", id, err)
	}
	if err := json.Unmarshal([]byte(apps), &st.EnforcedApps); err != nil {
		return st, false, fmt.Errorf("store: decode cost budget apps %q: %w", id, err)
	}
	st.UpdatedAt, _ = time.Parse(time.RFC3339, ts)
	return st, true, nil
}

// SaveCostBudgetState upserts the state of a budget.
func (db *DB) SaveCostBudgetState(ctx context.Context, st costbudget.State) error {
	apps := st.EnforcedApps
	if apps == nil {
		apps = []string{}
	}
	raw, err := json.Marshal(apps)
	if err != nil {
		return fmt.Errorf("store: encode cost budget apps: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO cost_budget_state (budget_id, period, highest, pct, spent, enforced, enforced_apps, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(budget_id) DO UPDATE SET period = excluded.period, highest = excluded.highest, pct = excluded.pct,
			spent = excluded.spent, enforced = excluded.enforced, enforced_apps = excluded.enforced_apps, updated_at = excluded.updated_at`,
		st.BudgetID, st.Period, st.Highest, st.Pct, st.Spent, st.Enforced, string(raw), st.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save cost budget state %q: %w", st.BudgetID, err)
	}
	return nil
}

// listCostBudgetEnforcements returns the apps held by a budget for period.
func (db *DB) listCostBudgetEnforcements(ctx context.Context, period string) ([]spendcap.Enforcement, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.enforced, s.enforced_apps, b.throttle_rps FROM cost_budget_state s
		JOIN cost_budgets b ON b.id = s.budget_id
		WHERE s.period = ? AND s.enforced != ''`, period)
	if err != nil {
		return nil, fmt.Errorf("store: list cost budget enforcements: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []spendcap.Enforcement
	for rows.Next() {
		var action, apps string
		var rps int
		if err := rows.Scan(&action, &apps, &rps); err != nil {
			return nil, fmt.Errorf("store: scan cost budget enforcement: %w", err)
		}
		var names []string
		if err := json.Unmarshal([]byte(apps), &names); err != nil {
			return nil, fmt.Errorf("store: decode cost budget enforcement apps: %w", err)
		}
		for _, n := range names {
			out = append(out, spendcap.Enforcement{App: n, Action: action, ThrottleRPS: rps})
		}
	}
	return out, rows.Err()
}

// mergeEnforcements keeps one entry per app: pause beats throttle, and the
// stricter throttle rate wins between two throttles.
func mergeEnforcements(lists ...[]spendcap.Enforcement) []spendcap.Enforcement {
	byApp := map[string]spendcap.Enforcement{}
	var order []string
	for _, l := range lists {
		for _, e := range l {
			cur, ok := byApp[e.App]
			if !ok {
				byApp[e.App] = e
				order = append(order, e.App)
				continue
			}
			switch {
			case cur.Action == spendcap.ActionPause:
			case e.Action == spendcap.ActionPause:
				byApp[e.App] = e
			case e.ThrottleRPS > 0 && (cur.ThrottleRPS == 0 || e.ThrottleRPS < cur.ThrottleRPS):
				byApp[e.App] = e
			}
		}
	}
	sort.Strings(order)
	out := make([]spendcap.Enforcement, 0, len(order))
	for _, a := range order {
		out = append(out, byApp[a])
	}
	return out
}
