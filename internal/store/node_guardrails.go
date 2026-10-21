package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NodeGuardrails holds one node's threshold overrides; a nil field uses the env default.
type NodeGuardrails struct {
	NodeID           string
	DiskWarnFreePct  *float64
	DiskCritFreePct  *float64
	InodeWarnFreePct *float64
	InodeCritFreePct *float64
	MemWarnAvailPct  *float64
	MemCritAvailPct  *float64
	SwapWarnUsedPct  *float64
	SwapCritUsedPct  *float64
	AutoClean        bool
	UpdatedAt        time.Time
	UpdatedBy        string
}

// NodeCleanupRun is one executed cleanup on a node.
type NodeCleanupRun struct {
	ID             string
	NodeID         string
	StartedAt      time.Time
	Trigger        string
	Actor          string
	ReclaimedBytes int64
	Body           string
	Error          string
}

// GetNodeGuardrails returns the node's overrides, ok false when none were saved.
func (db *DB) GetNodeGuardrails(ctx context.Context, nodeID string) (NodeGuardrails, bool, error) {
	var g NodeGuardrails
	var updated string
	var auto int
	err := db.QueryRowContext(ctx, `SELECT node_id, disk_warn_free_pct, disk_crit_free_pct, inode_warn_free_pct, inode_crit_free_pct,
		mem_warn_avail_pct, mem_crit_avail_pct, swap_warn_used_pct, swap_crit_used_pct, auto_clean, updated_at, updated_by
		FROM node_guardrails WHERE node_id = ?`, nodeID).Scan(&g.NodeID, &g.DiskWarnFreePct, &g.DiskCritFreePct, &g.InodeWarnFreePct,
		&g.InodeCritFreePct, &g.MemWarnAvailPct, &g.MemCritAvailPct, &g.SwapWarnUsedPct, &g.SwapCritUsedPct, &auto, &updated, &g.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeGuardrails{}, false, nil
	}
	if err != nil {
		return NodeGuardrails{}, false, fmt.Errorf("store: get node guardrails: %w", err)
	}
	g.AutoClean = auto != 0
	g.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return g, true, nil
}

// ListAutoCleanNodes returns the IDs of nodes with auto-clean switched on.
func (db *DB) ListAutoCleanNodes(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT node_id FROM node_guardrails WHERE auto_clean = 1 ORDER BY node_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list auto-clean nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan auto-clean node: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetNodeGuardrails upserts the node's overrides.
func (db *DB) SetNodeGuardrails(ctx context.Context, g NodeGuardrails, now time.Time) error {
	auto := 0
	if g.AutoClean {
		auto = 1
	}
	_, err := db.ExecContext(ctx, `INSERT INTO node_guardrails (node_id, disk_warn_free_pct, disk_crit_free_pct, inode_warn_free_pct, inode_crit_free_pct,
		mem_warn_avail_pct, mem_crit_avail_pct, swap_warn_used_pct, swap_crit_used_pct, auto_clean, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET disk_warn_free_pct = excluded.disk_warn_free_pct, disk_crit_free_pct = excluded.disk_crit_free_pct,
		inode_warn_free_pct = excluded.inode_warn_free_pct, inode_crit_free_pct = excluded.inode_crit_free_pct,
		mem_warn_avail_pct = excluded.mem_warn_avail_pct, mem_crit_avail_pct = excluded.mem_crit_avail_pct,
		swap_warn_used_pct = excluded.swap_warn_used_pct, swap_crit_used_pct = excluded.swap_crit_used_pct,
		auto_clean = excluded.auto_clean, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
		g.NodeID, g.DiskWarnFreePct, g.DiskCritFreePct, g.InodeWarnFreePct, g.InodeCritFreePct, g.MemWarnAvailPct, g.MemCritAvailPct,
		g.SwapWarnUsedPct, g.SwapCritUsedPct, auto, now.UTC().Format(time.RFC3339Nano), g.UpdatedBy)
	if err != nil {
		return fmt.Errorf("store: set node guardrails: %w", err)
	}
	return nil
}

// InsertNodeCleanupRun appends a run and trims the node's history to keep rows.
func (db *DB) InsertNodeCleanupRun(ctx context.Context, r NodeCleanupRun, keep int) error {
	_, err := db.ExecContext(ctx, `INSERT INTO node_cleanup_runs (id, node_id, started_at, trigger, actor, reclaimed_bytes, body, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.NodeID, r.StartedAt.UTC().Format(time.RFC3339Nano), r.Trigger, r.Actor, r.ReclaimedBytes, r.Body, r.Error)
	if err != nil {
		return fmt.Errorf("store: insert node cleanup run: %w", err)
	}
	if keep > 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM node_cleanup_runs WHERE node_id = ? AND id NOT IN
			(SELECT id FROM node_cleanup_runs WHERE node_id = ? ORDER BY started_at DESC LIMIT ?)`, r.NodeID, r.NodeID, keep); err != nil {
			return fmt.Errorf("store: trim node cleanup runs: %w", err)
		}
	}
	return nil
}

// ListNodeCleanupRuns returns the node's newest runs.
func (db *DB) ListNodeCleanupRuns(ctx context.Context, nodeID string, limit int) ([]NodeCleanupRun, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, node_id, started_at, trigger, actor, reclaimed_bytes, body, error
		FROM node_cleanup_runs WHERE node_id = ? ORDER BY started_at DESC LIMIT ?`, nodeID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list node cleanup runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeCleanupRun
	for rows.Next() {
		var r NodeCleanupRun
		var at string
		if err := rows.Scan(&r.ID, &r.NodeID, &at, &r.Trigger, &r.Actor, &r.ReclaimedBytes, &r.Body, &r.Error); err != nil {
			return nil, fmt.Errorf("store: scan node cleanup run: %w", err)
		}
		r.StartedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, r)
	}
	return out, rows.Err()
}
