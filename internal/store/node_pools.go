package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrPoolNotFound is returned when a pool name has no row.
var ErrPoolNotFound = errors.New("store: node pool not found")

// ErrPoolInUse is returned when deleting a pool an app still targets.
var ErrPoolInUse = errors.New("store: node pool is in use")

// NodePool is a named label selector over nodes.
type NodePool struct {
	Name     string            `json:"name"`
	Selector map[string]string `json:"selector"`
}

// SetNodePool creates a pool or replaces its selector.
func (db *DB) SetNodePool(ctx context.Context, name string, selector map[string]string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set node pool: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO node_pools (name, created_at) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`, name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("store: set node pool %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM node_pool_labels WHERE pool = ?`, name); err != nil {
		return fmt.Errorf("store: set node pool %q: clear selector: %w", name, err)
	}
	for k, v := range selector {
		if _, err := tx.ExecContext(ctx, `INSERT INTO node_pool_labels (pool, key, value) VALUES (?, ?, ?)`, name, k, v); err != nil {
			return fmt.Errorf("store: set node pool %q: insert %q: %w", name, k, err)
		}
	}
	return tx.Commit()
}

// ListNodePools returns every pool with its selector, sorted by name.
func (db *DB) ListNodePools(ctx context.Context) ([]NodePool, error) {
	rows, err := db.QueryContext(ctx, `SELECT p.name, COALESCE(l.key, ''), COALESCE(l.value, '') FROM node_pools p LEFT JOIN node_pool_labels l ON l.pool = p.name ORDER BY p.name, l.key`)
	if err != nil {
		return nil, fmt.Errorf("store: list node pools: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byName := map[string]*NodePool{}
	for rows.Next() {
		var n, k, v string
		if err := rows.Scan(&n, &k, &v); err != nil {
			return nil, fmt.Errorf("store: scan node pool: %w", err)
		}
		p := byName[n]
		if p == nil {
			p = &NodePool{Name: n, Selector: map[string]string{}}
			byName[n] = p
		}
		if k != "" {
			p.Selector[k] = v
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]NodePool, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetNodePool returns one pool or ErrPoolNotFound.
func (db *DB) GetNodePool(ctx context.Context, name string) (NodePool, error) {
	all, err := db.ListNodePools(ctx)
	if err != nil {
		return NodePool{}, err
	}
	for _, p := range all {
		if p.Name == name {
			return p, nil
		}
	}
	return NodePool{}, ErrPoolNotFound
}

// DeleteNodePool removes a pool, refusing while an app targets it.
func (db *DB) DeleteNodePool(ctx context.Context, name string) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_pools WHERE pool = ?`, name).Scan(&n); err != nil {
		return fmt.Errorf("store: delete node pool %q: %w", name, err)
	}
	if n > 0 {
		return ErrPoolInUse
	}
	res, err := db.ExecContext(ctx, `DELETE FROM node_pools WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete node pool %q: %w", name, err)
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return ErrPoolNotFound
	}
	return nil
}

// SetAppPool points an app at a pool; an empty pool clears it.
func (db *DB) SetAppPool(ctx context.Context, app, pool string) error {
	if pool == "" {
		_, err := db.ExecContext(ctx, `DELETE FROM app_pools WHERE app = ?`, app)
		return err
	}
	if _, err := db.GetNodePool(ctx, pool); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO app_pools (app, pool) VALUES (?, ?) ON CONFLICT(app) DO UPDATE SET pool = excluded.pool`, app, pool)
	return err
}

// GetAppPool returns the pool an app targets, empty when none.
func (db *DB) GetAppPool(ctx context.Context, app string) (string, error) {
	var pool string
	err := db.QueryRowContext(ctx, `SELECT pool FROM app_pools WHERE app = ?`, app).Scan(&pool)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return pool, err
}

// WarmTargets says where an app's image is kept pulled: explicit nodes, nodes
// carrying every label, or both.
type WarmTargets struct {
	Nodes  []string          `json:"nodes"`
	Labels map[string]string `json:"labels"`
}

// SetAppWarm replaces an app's warm targets.
func (db *DB) SetAppWarm(ctx context.Context, app string, t WarmTargets) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set app warm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_warm WHERE app = ?`, app); err != nil {
		return fmt.Errorf("store: set app warm %q: clear: %w", app, err)
	}
	for _, n := range t.Nodes {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO app_warm (app, kind, key, value) VALUES (?, 'node', ?, '')`, app, n); err != nil {
			return fmt.Errorf("store: set app warm %q: node %q: %w", app, n, err)
		}
	}
	for k, v := range t.Labels {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_warm (app, kind, key, value) VALUES (?, 'label', ?, ?)`, app, k, v); err != nil {
			return fmt.Errorf("store: set app warm %q: label %q: %w", app, k, err)
		}
	}
	return tx.Commit()
}

// GetAppWarm returns an app's warm targets, empty (not nil) when none.
func (db *DB) GetAppWarm(ctx context.Context, app string) (WarmTargets, error) {
	out := WarmTargets{Nodes: []string{}, Labels: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT kind, key, value FROM app_warm WHERE app = ? ORDER BY kind, key`, app)
	if err != nil {
		return out, fmt.Errorf("store: get app warm %q: %w", app, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var kind, k, v string
		if err := rows.Scan(&kind, &k, &v); err != nil {
			return out, fmt.Errorf("store: scan app warm: %w", err)
		}
		if kind == "node" {
			out.Nodes = append(out.Nodes, k)
		} else {
			out.Labels[k] = v
		}
	}
	return out, rows.Err()
}
