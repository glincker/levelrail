package store

import (
	"context"
	"errors"
	"fmt"
)

// Private network errors.
var (
	ErrPrivateNetworkNotFound     = errors.New("store: private network not found")
	ErrPrivateNetworkRuleNotFound = errors.New("store: private network rule not found")
)

// PrivateNetwork is a named Docker network a set of apps join. DefaultDeny
// means members are isolated from each other until a rule allows a pair.
type PrivateNetwork struct {
	ID          string
	Name        string
	ProjectID   string
	DefaultDeny bool
	CreatedAt   string
	Members     []string
}

// PrivateNetworkRule allows traffic between two apps across networks.
type PrivateNetworkRule struct {
	ID        string
	FromApp   string
	ToApp     string
	Note      string
	CreatedAt string
}

func pnBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SavePrivateNetwork inserts a private network with its members.
func (db *DB) SavePrivateNetwork(ctx context.Context, n PrivateNetwork) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin save private network: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO private_networks (id, name, project_id, default_deny, created_at) VALUES (?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.ProjectID, pnBool(n.DefaultDeny), n.CreatedAt); err != nil {
		return fmt.Errorf("store: save private network %q: %w", n.Name, err)
	}
	for _, app := range n.Members {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO private_network_members (network_id, app_id) VALUES (?, ?)`, n.ID, app); err != nil {
			return fmt.Errorf("store: save private network member: %w", err)
		}
	}
	return tx.Commit()
}

// UpdatePrivateNetwork replaces the default-deny flag and member set.
func (db *DB) UpdatePrivateNetwork(ctx context.Context, id string, defaultDeny bool, members []string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin update private network: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE private_networks SET default_deny = ? WHERE id = ?`, pnBool(defaultDeny), id)
	if err != nil {
		return fmt.Errorf("store: update private network %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPrivateNetworkNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM private_network_members WHERE network_id = ?`, id); err != nil {
		return fmt.Errorf("store: clear private network members: %w", err)
	}
	for _, app := range members {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO private_network_members (network_id, app_id) VALUES (?, ?)`, id, app); err != nil {
			return fmt.Errorf("store: save private network member: %w", err)
		}
	}
	return tx.Commit()
}

// DeletePrivateNetwork removes a network and its memberships.
func (db *DB) DeletePrivateNetwork(ctx context.Context, id string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin delete private network: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM private_network_members WHERE network_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete private network members: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM private_networks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete private network %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPrivateNetworkNotFound
	}
	return tx.Commit()
}

// ListPrivateNetworks returns every private network ordered by name, with members.
func (db *DB) ListPrivateNetworks(ctx context.Context) ([]PrivateNetwork, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, project_id, default_deny, created_at FROM private_networks ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list private networks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PrivateNetwork
	idx := map[string]int{}
	for rows.Next() {
		var n PrivateNetwork
		var deny int
		if err := rows.Scan(&n.ID, &n.Name, &n.ProjectID, &deny, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan private network: %w", err)
		}
		n.DefaultDeny = deny != 0
		idx[n.ID] = len(out)
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate private networks: %w", err)
	}
	mrows, err := db.QueryContext(ctx, `SELECT network_id, app_id FROM private_network_members ORDER BY app_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list private network members: %w", err)
	}
	defer func() { _ = mrows.Close() }()
	for mrows.Next() {
		var nid, app string
		if err := mrows.Scan(&nid, &app); err != nil {
			return nil, fmt.Errorf("store: scan private network member: %w", err)
		}
		if i, ok := idx[nid]; ok {
			out[i].Members = append(out[i].Members, app)
		}
	}
	return out, mrows.Err()
}

// GetPrivateNetworkByName returns the network with this name or ID.
func (db *DB) GetPrivateNetworkByName(ctx context.Context, name string) (PrivateNetwork, error) {
	all, err := db.ListPrivateNetworks(ctx)
	if err != nil {
		return PrivateNetwork{}, err
	}
	for _, n := range all {
		if n.Name == name || n.ID == name {
			return n, nil
		}
	}
	return PrivateNetwork{}, ErrPrivateNetworkNotFound
}

// SavePrivateNetworkRule inserts an allow rule, a no-op if the pair exists.
func (db *DB) SavePrivateNetworkRule(ctx context.Context, r PrivateNetworkRule) error {
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO private_network_rules (id, from_app, to_app, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		r.ID, r.FromApp, r.ToApp, r.Note, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save private network rule: %w", err)
	}
	return nil
}

// DeletePrivateNetworkRule removes a rule by ID.
func (db *DB) DeletePrivateNetworkRule(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM private_network_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete private network rule %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrPrivateNetworkRuleNotFound
	}
	return nil
}

// ListPrivateNetworkRules returns every rule ordered by creation.
func (db *DB) ListPrivateNetworkRules(ctx context.Context) ([]PrivateNetworkRule, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, from_app, to_app, note, created_at FROM private_network_rules ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list private network rules: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PrivateNetworkRule
	for rows.Next() {
		var r PrivateNetworkRule
		if err := rows.Scan(&r.ID, &r.FromApp, &r.ToApp, &r.Note, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan private network rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
