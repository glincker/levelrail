package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/usagecost"
)

// ErrUsageBudgetNotFound is returned when a budget id matches no row.
var ErrUsageBudgetNotFound = errors.New("store: usage budget not found")

// GetUsageCostSettings returns the saved pricing, or the defaults.
func (db *DB) GetUsageCostSettings(ctx context.Context) (usagecost.Settings, error) {
	s := usagecost.DefaultSettings()
	err := db.QueryRowContext(ctx, `
		SELECT mode, currency, cpu_core_hour, mem_gib_hour, storage_gib_month, egress_gib, idle_spread_pct, basis, cpu_weight_pct
		FROM usage_cost_settings WHERE id = 1`).
		Scan(&s.Mode, &s.Currency, &s.CPUCoreHour, &s.MemGiBHour, &s.StorageGiBMonth, &s.EgressGiB, &s.IdleSpreadPct, &s.Basis, &s.CPUWeightPct)
	if errors.Is(err, sql.ErrNoRows) {
		return usagecost.DefaultSettings(), nil
	}
	if err != nil {
		return s, fmt.Errorf("store: get usage cost settings: %w", err)
	}
	return s, nil
}

// SaveUsageCostSettings replaces the pricing configuration.
func (db *DB) SaveUsageCostSettings(ctx context.Context, s usagecost.Settings, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO usage_cost_settings (id, mode, currency, cpu_core_hour, mem_gib_hour, storage_gib_month, egress_gib, idle_spread_pct, basis, cpu_weight_pct, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET mode = excluded.mode, currency = excluded.currency, cpu_core_hour = excluded.cpu_core_hour,
			mem_gib_hour = excluded.mem_gib_hour, storage_gib_month = excluded.storage_gib_month, egress_gib = excluded.egress_gib,
			idle_spread_pct = excluded.idle_spread_pct, basis = excluded.basis, cpu_weight_pct = excluded.cpu_weight_pct, updated_at = excluded.updated_at`,
		s.Mode, s.Currency, s.CPUCoreHour, s.MemGiBHour, s.StorageGiBMonth, s.EgressGiB, s.IdleSpreadPct, s.Basis, s.CPUWeightPct,
		now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: save usage cost settings: %w", err)
	}
	return nil
}

// ListUsageCostNodes returns every configured node bill by node id.
func (db *DB) ListUsageCostNodes(ctx context.Context) (map[string]usagecost.NodePrice, error) {
	rows, err := db.QueryContext(ctx, `SELECT node_id, monthly_price, vcpus, memory_gib FROM usage_cost_nodes`)
	if err != nil {
		return nil, fmt.Errorf("store: list usage cost nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]usagecost.NodePrice{}
	for rows.Next() {
		var n usagecost.NodePrice
		if err := rows.Scan(&n.NodeID, &n.MonthlyPrice, &n.VCPUs, &n.MemoryGiB); err != nil {
			return nil, fmt.Errorf("store: scan usage cost node: %w", err)
		}
		out[n.NodeID] = n
	}
	return out, rows.Err()
}

// ReplaceUsageCostNodes saves the full set of node bills.
func (db *DB) ReplaceUsageCostNodes(ctx context.Context, nodes []usagecost.NodePrice, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace usage cost nodes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_cost_nodes`); err != nil {
		return fmt.Errorf("store: clear usage cost nodes: %w", err)
	}
	ts := now.UTC().Format(time.RFC3339)
	for _, n := range nodes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO usage_cost_nodes (node_id, monthly_price, vcpus, memory_gib, updated_at) VALUES (?, ?, ?, ?, ?)`,
			n.NodeID, n.MonthlyPrice, n.VCPUs, n.MemoryGiB, ts); err != nil {
			return fmt.Errorf("store: save usage cost node %q: %w", n.NodeID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit usage cost nodes: %w", err)
	}
	return nil
}

// SaveUsageDay stores one day's allocated rows, replacing any earlier
// snapshot of that day. The caller adds a KindDay marker row.
func (db *DB) SaveUsageDay(ctx context.Context, day string, rows []usagecost.DailyCost) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save usage day %s: %w", day, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM usage_cost_daily WHERE day = ?`, day); err != nil {
		return fmt.Errorf("store: clear usage day %s: %w", day, err)
	}
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO usage_cost_daily (day, kind, name, team_id, project_id, environment_id, node_id, up_seconds, cpu_avg_cores, cpu_p50_cores,
				mem_avg_bytes, mem_p50_bytes, storage_bytes, egress_bytes, compute_cost, storage_cost, egress_cost, idle_cost)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			day, r.Kind, r.Name, r.TeamID, r.ProjectID, r.EnvironmentID, r.NodeID, r.UpSeconds, r.CPUAvgCores, r.CPUP50Cores,
			r.MemAvgBytes, r.MemP50Bytes, r.StorageBytes, r.EgressBytes, r.ComputeCost, r.StorageCost, r.EgressCost, r.IdleCost); err != nil {
			return fmt.Errorf("store: insert usage row %s/%s: %w", day, r.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit usage day %s: %w", day, err)
	}
	return nil
}

// ListUsageDays returns the stored rows with from <= day < to, markers
// included.
func (db *DB) ListUsageDays(ctx context.Context, from, to string) ([]usagecost.DailyCost, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT day, kind, name, team_id, project_id, environment_id, node_id, up_seconds, cpu_avg_cores, cpu_p50_cores,
			mem_avg_bytes, mem_p50_bytes, storage_bytes, egress_bytes, compute_cost, storage_cost, egress_cost, idle_cost
		FROM usage_cost_daily WHERE day >= ? AND day < ? ORDER BY day, kind, name`, from, to)
	if err != nil {
		return nil, fmt.Errorf("store: list usage days: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []usagecost.DailyCost
	for rows.Next() {
		var r usagecost.DailyCost
		if err := rows.Scan(&r.Day, &r.Kind, &r.Name, &r.TeamID, &r.ProjectID, &r.EnvironmentID, &r.NodeID, &r.UpSeconds,
			&r.CPUAvgCores, &r.CPUP50Cores, &r.MemAvgBytes, &r.MemP50Bytes, &r.StorageBytes, &r.EgressBytes,
			&r.ComputeCost, &r.StorageCost, &r.EgressCost, &r.IdleCost); err != nil {
			return nil, fmt.Errorf("store: scan usage row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListUsageBudgets returns every budget.
func (db *DB) ListUsageBudgets(ctx context.Context) ([]usagecost.Budget, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, scope, scope_id, monthly, warn_pct FROM usage_budgets ORDER BY scope, scope_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list usage budgets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []usagecost.Budget
	for rows.Next() {
		var b usagecost.Budget
		if err := rows.Scan(&b.ID, &b.Scope, &b.ScopeID, &b.Monthly, &b.WarnPct); err != nil {
			return nil, fmt.Errorf("store: scan usage budget: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// SaveUsageBudget upserts the single budget of a scope and returns it.
func (db *DB) SaveUsageBudget(ctx context.Context, b usagecost.Budget, now time.Time) (usagecost.Budget, error) {
	ts := now.UTC().Format(time.RFC3339)
	if b.ID == "" {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return b, fmt.Errorf("store: generate usage budget id: %w", err)
		}
		b.ID = "bud_" + hex.EncodeToString(buf[:])
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO usage_budgets (id, scope, scope_id, monthly, warn_pct, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope, scope_id) DO UPDATE SET monthly = excluded.monthly, warn_pct = excluded.warn_pct, updated_at = excluded.updated_at`,
		b.ID, b.Scope, b.ScopeID, b.Monthly, b.WarnPct, ts, ts)
	if err != nil {
		return b, fmt.Errorf("store: save usage budget: %w", err)
	}
	err = db.QueryRowContext(ctx, `SELECT id FROM usage_budgets WHERE scope = ? AND scope_id = ?`, b.Scope, b.ScopeID).Scan(&b.ID)
	if err != nil {
		return b, fmt.Errorf("store: reload usage budget: %w", err)
	}
	return b, nil
}

// DeleteUsageBudget removes a budget.
func (db *DB) DeleteUsageBudget(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM usage_budgets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete usage budget %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUsageBudgetNotFound
	}
	return nil
}

// GetUsageBudget returns one budget.
func (db *DB) GetUsageBudget(ctx context.Context, id string) (usagecost.Budget, error) {
	var b usagecost.Budget
	err := db.QueryRowContext(ctx, `SELECT id, scope, scope_id, monthly, warn_pct FROM usage_budgets WHERE id = ?`, id).
		Scan(&b.ID, &b.Scope, &b.ScopeID, &b.Monthly, &b.WarnPct)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrUsageBudgetNotFound
	}
	if err != nil {
		return b, fmt.Errorf("store: get usage budget %q: %w", id, err)
	}
	return b, nil
}
