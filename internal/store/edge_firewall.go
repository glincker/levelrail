package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrEdgeFirewallRuleNotFound is returned when a rule id matches nothing.
var ErrEdgeFirewallRuleNotFound = errors.New("store: firewall rule not found")

// EdgeFirewallRule is one stored ingress firewall rule. Conditions is the
// JSON list the API validated; the store treats it as opaque.
type EdgeFirewallRule struct {
	ID                string
	Name              string
	Note              string
	Scope             string
	Enabled           bool
	Position          int
	Action            string
	Conditions        string
	RateCount         int
	RateWindowSeconds int
	CreatedAt         string
	UpdatedAt         string
}

const edgeFirewallColumns = `id, name, note, scope, enabled, position, action, conditions, rate_count, rate_window_seconds, created_at, updated_at`

type edgeRowScanner interface{ Scan(dest ...any) error }

func scanEdgeFirewallRule(s edgeRowScanner) (EdgeFirewallRule, error) {
	var r EdgeFirewallRule
	var enabled int
	err := s.Scan(&r.ID, &r.Name, &r.Note, &r.Scope, &enabled, &r.Position, &r.Action, &r.Conditions, &r.RateCount, &r.RateWindowSeconds, &r.CreatedAt, &r.UpdatedAt)
	r.Enabled = enabled != 0
	return r, err
}

// ListEdgeFirewallRules returns every rule in evaluation order.
func (db *DB) ListEdgeFirewallRules(ctx context.Context) ([]EdgeFirewallRule, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+edgeFirewallColumns+` FROM edge_firewall_rules ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list firewall rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EdgeFirewallRule
	for rows.Next() {
		r, err := scanEdgeFirewallRule(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan firewall rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetEdgeFirewallRule returns one rule.
func (db *DB) GetEdgeFirewallRule(ctx context.Context, id string) (EdgeFirewallRule, error) {
	r, err := scanEdgeFirewallRule(db.QueryRowContext(ctx, `SELECT `+edgeFirewallColumns+` FROM edge_firewall_rules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return EdgeFirewallRule{}, ErrEdgeFirewallRuleNotFound
	}
	if err != nil {
		return EdgeFirewallRule{}, fmt.Errorf("store: get firewall rule %q: %w", id, err)
	}
	return r, nil
}

// CountEdgeFirewallRules returns how many rules exist.
func (db *DB) CountEdgeFirewallRules(ctx context.Context) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edge_firewall_rules`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count firewall rules: %w", err)
	}
	return n, nil
}

// CreateEdgeFirewallRule inserts r. A Position of zero or less appends the
// rule after the last one.
func (db *DB) CreateEdgeFirewallRule(ctx context.Context, r EdgeFirewallRule) (EdgeFirewallRule, error) {
	if r.Position <= 0 {
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) + 1 FROM edge_firewall_rules`).Scan(&r.Position); err != nil {
			return EdgeFirewallRule{}, fmt.Errorf("store: next firewall position: %w", err)
		}
	}
	_, err := db.ExecContext(ctx, `INSERT INTO edge_firewall_rules (`+edgeFirewallColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Note, r.Scope, boolInt(r.Enabled), r.Position, r.Action, r.Conditions, r.RateCount, r.RateWindowSeconds, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return EdgeFirewallRule{}, fmt.Errorf("store: create firewall rule: %w", err)
	}
	return r, nil
}

// UpdateEdgeFirewallRule replaces the editable fields of the rule r.ID
// names; its position and creation time stay.
func (db *DB) UpdateEdgeFirewallRule(ctx context.Context, r EdgeFirewallRule) error {
	res, err := db.ExecContext(ctx, `UPDATE edge_firewall_rules SET name = ?, note = ?, scope = ?, enabled = ?, action = ?, conditions = ?, rate_count = ?, rate_window_seconds = ?, updated_at = ? WHERE id = ?`,
		r.Name, r.Note, r.Scope, boolInt(r.Enabled), r.Action, r.Conditions, r.RateCount, r.RateWindowSeconds, r.UpdatedAt, r.ID)
	if err != nil {
		return fmt.Errorf("store: update firewall rule %q: %w", r.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEdgeFirewallRuleNotFound
	}
	return nil
}

// DeleteEdgeFirewallRule removes a rule.
func (db *DB) DeleteEdgeFirewallRule(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM edge_firewall_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete firewall rule %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEdgeFirewallRuleNotFound
	}
	return nil
}

// ReorderEdgeFirewallRules renumbers positions 1..n following ids, which
// must name every existing rule exactly once.
func (db *DB) ReorderEdgeFirewallRules(ctx context.Context, ids []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin firewall reorder: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM edge_firewall_rules`).Scan(&total); err != nil {
		return fmt.Errorf("store: count firewall rules: %w", err)
	}
	if total != len(ids) {
		return fmt.Errorf("store: firewall reorder names %d rules, %d exist", len(ids), total)
	}
	for i, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE edge_firewall_rules SET position = ? WHERE id = ?`, i+1, id)
		if err != nil {
			return fmt.Errorf("store: reorder firewall rule %q: %w", id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrEdgeFirewallRuleNotFound
		}
	}
	return tx.Commit()
}

// GetEdgeFirewallSettings returns the settings JSON.
func (db *DB) GetEdgeFirewallSettings(ctx context.Context) (string, error) {
	var s string
	if err := db.QueryRowContext(ctx, `SELECT settings FROM edge_firewall_settings WHERE id = 1`).Scan(&s); err != nil {
		return "", fmt.Errorf("store: get firewall settings: %w", err)
	}
	return s, nil
}

// SetEdgeFirewallSettings replaces the settings JSON.
func (db *DB) SetEdgeFirewallSettings(ctx context.Context, settings string) error {
	if _, err := db.ExecContext(ctx, `UPDATE edge_firewall_settings SET settings = ? WHERE id = 1`, settings); err != nil {
		return fmt.Errorf("store: set firewall settings: %w", err)
	}
	return nil
}
