package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// NodeThresholdOverride holds one node's alert threshold overrides; a nil field uses the platform default.
type NodeThresholdOverride struct {
	NodeID        string
	CPUPercent    *int
	MemoryPercent *int
	DiskPercent   *int
	UpdatedAt     time.Time
}

// Empty reports whether no field is overridden, so the row can be dropped.
func (o NodeThresholdOverride) Empty() bool {
	return o.CPUPercent == nil && o.MemoryPercent == nil && o.DiskPercent == nil
}

func scanNodeThresholdOverride(scan func(...any) error) (NodeThresholdOverride, error) {
	var (
		o             NodeThresholdOverride
		cpu, mem, dsk sql.NullInt64
		updated       string
	)
	if err := scan(&o.NodeID, &cpu, &mem, &dsk, &updated); err != nil {
		return o, err
	}
	o.CPUPercent, o.MemoryPercent, o.DiskPercent = nullIntPtr(cpu), nullIntPtr(mem), nullIntPtr(dsk)
	o.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return o, nil
}

func intPtrArg(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

const nodeThresholdSelect = `SELECT node_id, cpu_percent, memory_percent, disk_percent, updated_at FROM node_threshold_overrides`

// GetNodeThresholdOverride returns a node's overrides; a node without a row gets an empty override.
func (db *DB) GetNodeThresholdOverride(ctx context.Context, nodeID string) (NodeThresholdOverride, error) {
	o, err := scanNodeThresholdOverride(db.QueryRowContext(ctx, nodeThresholdSelect+` WHERE node_id = ?`, nodeID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeThresholdOverride{NodeID: nodeID}, nil
	}
	if err != nil {
		return o, fmt.Errorf("store: get node threshold override %q: %w", nodeID, err)
	}
	return o, nil
}

// ListNodeThresholdOverrides returns every stored override keyed by node ID.
func (db *DB) ListNodeThresholdOverrides(ctx context.Context) (map[string]NodeThresholdOverride, error) {
	rows, err := db.QueryContext(ctx, nodeThresholdSelect)
	if err != nil {
		return nil, fmt.Errorf("store: list node threshold overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]NodeThresholdOverride{}
	for rows.Next() {
		o, err := scanNodeThresholdOverride(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node threshold override: %w", err)
		}
		out[o.NodeID] = o
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list node threshold overrides: %w", err)
	}
	return out, nil
}

// SetNodeThresholdOverride upserts a node's overrides; an all-nil override deletes the row.
func (db *DB) SetNodeThresholdOverride(ctx context.Context, o NodeThresholdOverride) error {
	if _, err := db.GetNode(ctx, o.NodeID); err != nil {
		return err
	}
	if o.Empty() {
		if _, err := db.ExecContext(ctx, `DELETE FROM node_threshold_overrides WHERE node_id = ?`, o.NodeID); err != nil {
			return fmt.Errorf("store: clear node threshold override %q: %w", o.NodeID, err)
		}
		return nil
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_threshold_overrides (node_id, cpu_percent, memory_percent, disk_percent, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET cpu_percent = excluded.cpu_percent, memory_percent = excluded.memory_percent,
			disk_percent = excluded.disk_percent, updated_at = excluded.updated_at
	`, o.NodeID, intPtrArg(o.CPUPercent), intPtrArg(o.MemoryPercent), intPtrArg(o.DiskPercent), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set node threshold override %q: %w", o.NodeID, err)
	}
	return nil
}
