package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrFirewallRuleNotFound is returned by DeleteFirewallRule when id
// doesn't match any row.
var ErrFirewallRuleNotFound = errors.New("store: firewall rule not found")

// FirewallRuleAction is the ufw action a FirewallRule applies.
type FirewallRuleAction string

// The two FirewallRuleAction values a rule can hold.
const (
	FirewallRuleActionAllow FirewallRuleAction = "allow"
	FirewallRuleActionDeny  FirewallRuleAction = "deny"
)

// FirewallRule is one declarative host firewall rule
// internal/reconcile/firewall converges onto ufw.
type FirewallRule struct {
	ID string
	// NodeID is '' for the control plane's own local node, the only
	// value the reconciler currently acts on (see migration 0262's own
	// doc comment).
	NodeID     string
	Port       int
	Protocol   string
	SourceCIDR string
	Action     FirewallRuleAction
	Label      string
	CreatedAt  string
}

// SaveFirewallRule inserts a new firewall rule row. IDs are minted by
// the caller before this call, the same "generate before the INSERT"
// pattern RegistryCredential uses.
func (db *DB) SaveFirewallRule(ctx context.Context, r FirewallRule) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO firewall_rules (id, node_id, port, protocol, source_cidr, action, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.NodeID, r.Port, r.Protocol, r.SourceCIDR, string(r.Action), r.Label, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save firewall rule %q: %w", r.ID, err)
	}
	return nil
}

// GetFirewallRule returns the firewall rule with this ID, or
// ErrFirewallRuleNotFound.
func (db *DB) GetFirewallRule(ctx context.Context, id string) (FirewallRule, error) {
	var r FirewallRule
	var action string
	err := db.QueryRowContext(ctx, `
		SELECT id, node_id, port, protocol, source_cidr, action, label, created_at
		FROM firewall_rules
		WHERE id = ?
	`, id).Scan(&r.ID, &r.NodeID, &r.Port, &r.Protocol, &r.SourceCIDR, &action, &r.Label, &r.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return FirewallRule{}, ErrFirewallRuleNotFound
	}
	if err != nil {
		return FirewallRule{}, fmt.Errorf("store: get firewall rule %q: %w", id, err)
	}
	r.Action = FirewallRuleAction(action)
	return r, nil
}

// ListFirewallRules returns every firewall rule, oldest first.
func (db *DB) ListFirewallRules(ctx context.Context) ([]FirewallRule, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, node_id, port, protocol, source_cidr, action, label, created_at
		FROM firewall_rules
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list firewall rules: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []FirewallRule
	for rows.Next() {
		var r FirewallRule
		var action string
		if err := rows.Scan(&r.ID, &r.NodeID, &r.Port, &r.Protocol, &r.SourceCIDR, &action, &r.Label, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan firewall rule row: %w", err)
		}
		r.Action = FirewallRuleAction(action)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate firewall rule rows: %w", err)
	}
	return out, nil
}

// DeleteFirewallRule removes a firewall rule row. It does not itself
// touch the host's ufw state: internal/reconcile/firewall's next pass
// removes the matching rule from ufw once it sees it's no longer
// desired, the same "store is desired state, the reconciler converges
// observed state to it" split every other reconciled resource follows.
// Returns ErrFirewallRuleNotFound if id doesn't exist.
func (db *DB) DeleteFirewallRule(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM firewall_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete firewall rule %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrFirewallRuleNotFound, "delete firewall rule %q", id)
}
