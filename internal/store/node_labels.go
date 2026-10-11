package store

import (
	"context"
	"fmt"
)

// ListNodeLabels returns every node's labels keyed by node ID.
func (db *DB) ListNodeLabels(ctx context.Context) (map[string]map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT node_id, key, value FROM node_labels ORDER BY node_id, key`)
	if err != nil {
		return nil, fmt.Errorf("store: list node labels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]string{}
	for rows.Next() {
		var id, k, v string
		if err := rows.Scan(&id, &k, &v); err != nil {
			return nil, fmt.Errorf("store: scan node label: %w", err)
		}
		if out[id] == nil {
			out[id] = map[string]string{}
		}
		out[id][k] = v
	}
	return out, rows.Err()
}

// GetNodeLabels returns one node's labels, empty (not nil) when it has none.
func (db *DB) GetNodeLabels(ctx context.Context, nodeID string) (map[string]string, error) {
	all, err := db.ListNodeLabels(ctx)
	if err != nil {
		return nil, err
	}
	if l := all[nodeID]; l != nil {
		return l, nil
	}
	return map[string]string{}, nil
}

// SetNodeLabels replaces a node's whole label set.
func (db *DB) SetNodeLabels(ctx context.Context, nodeID string, labels map[string]string) error {
	if _, err := db.GetNode(ctx, nodeID); err != nil {
		return err
	}
	return db.replaceKV(ctx, `DELETE FROM node_labels WHERE node_id = ?`, []any{nodeID},
		`INSERT INTO node_labels (node_id, key, value) VALUES (?, ?, ?)`, []any{nodeID}, labels)
}

// GetPlacementSelector returns the selector of one app or database, empty when unset.
func (db *DB) GetPlacementSelector(ctx context.Context, kind, resource string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM placement_selectors WHERE kind = ? AND resource = ?`, kind, resource)
	if err != nil {
		return nil, fmt.Errorf("store: get placement selector %s/%s: %w", kind, resource, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("store: scan placement selector: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetPlacementSelector replaces the selector of one app or database.
func (db *DB) SetPlacementSelector(ctx context.Context, kind, resource string, selector map[string]string) error {
	return db.replaceKV(ctx, `DELETE FROM placement_selectors WHERE kind = ? AND resource = ?`, []any{kind, resource},
		`INSERT INTO placement_selectors (kind, resource, key, value) VALUES (?, ?, ?, ?)`, []any{kind, resource}, selector)
}

func (db *DB) replaceKV(ctx context.Context, del string, delArgs []any, ins string, insPrefix []any, kv map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace labels: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, del, delArgs...); err != nil {
		return fmt.Errorf("store: replace labels: delete: %w", err)
	}
	for k, v := range kv {
		args := append(append([]any{}, insPrefix...), k, v)
		if _, err := tx.ExecContext(ctx, ins, args...); err != nil {
			return fmt.Errorf("store: replace labels: insert %q: %w", k, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: replace labels: commit: %w", err)
	}
	return nil
}
