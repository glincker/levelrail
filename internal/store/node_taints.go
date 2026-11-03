package store

import (
	"context"
	"fmt"
)

// NodeTaint is one taint on a node. The effect is one of NoSchedule,
// PreferNoSchedule or NoExecute; the placement package owns the meaning.
type NodeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	Effect string `json:"effect"`
}

// ListNodeTaints returns every node's taints keyed by node ID.
func (db *DB) ListNodeTaints(ctx context.Context) (map[string][]NodeTaint, error) {
	rows, err := db.QueryContext(ctx, `SELECT node_id, key, value, effect FROM node_taints ORDER BY node_id, key, effect`)
	if err != nil {
		return nil, fmt.Errorf("store: list node taints: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string][]NodeTaint{}
	for rows.Next() {
		var id string
		var t NodeTaint
		if err := rows.Scan(&id, &t.Key, &t.Value, &t.Effect); err != nil {
			return nil, fmt.Errorf("store: scan node taint: %w", err)
		}
		out[id] = append(out[id], t)
	}
	return out, rows.Err()
}

// GetNodeTaints returns one node's taints, empty (not nil) when it has none.
func (db *DB) GetNodeTaints(ctx context.Context, nodeID string) ([]NodeTaint, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value, effect FROM node_taints WHERE node_id = ? ORDER BY key, effect`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("store: get node taints: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []NodeTaint{}
	for rows.Next() {
		var t NodeTaint
		if err := rows.Scan(&t.Key, &t.Value, &t.Effect); err != nil {
			return nil, fmt.Errorf("store: scan node taint: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetNodeTaints replaces a node's whole taint set in one transaction.
func (db *DB) SetNodeTaints(ctx context.Context, nodeID string, taints []NodeTaint) error {
	if _, err := db.GetNode(ctx, nodeID); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set node taints: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_taints WHERE node_id = ?`, nodeID); err != nil {
		return fmt.Errorf("store: set node taints: delete: %w", err)
	}
	for _, t := range taints {
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_taints (node_id, key, value, effect) VALUES (?, ?, ?, ?)`, nodeID, t.Key, t.Value, t.Effect); err != nil {
			return fmt.Errorf("store: set node taints: insert %q: %w", t.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set node taints: commit: %w", err)
	}
	return nil
}
