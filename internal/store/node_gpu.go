package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
)

// NodeGPU is a node's stored GPU snapshot.
type NodeGPU struct {
	NodeID    string
	Info      gpu.Info
	UpdatedAt time.Time
}

type gpuDeviceJSON struct {
	Index        int    `json:"index"`
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	VRAMTotalMiB int64  `json:"vram_total_mib"`
	VRAMUsedMiB  int64  `json:"vram_used_mib"`
	Utilization  int    `json:"utilization_percent"`
}

// SetNodeGPU upserts a node's GPU snapshot.
func (db *DB) SetNodeGPU(ctx context.Context, nodeID string, info gpu.Info) error {
	devs := make([]gpuDeviceJSON, len(info.Devices))
	for i, d := range info.Devices {
		devs[i] = gpuDeviceJSON{d.Index, d.UUID, d.Name, d.VRAMTotalMiB, d.VRAMUsedMiB, d.UtilizationPercent}
	}
	raw, err := json.Marshal(devs)
	if err != nil {
		return fmt.Errorf("store: marshal node gpu devices: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_gpus (node_id, present, driver_version, runtime_installed, devices, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET present = excluded.present, driver_version = excluded.driver_version,
			runtime_installed = excluded.runtime_installed, devices = excluded.devices, updated_at = excluded.updated_at
	`, nodeID, boolToInt(info.Present), info.DriverVersion, boolToInt(info.RuntimeInstalled), string(raw), formatTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("store: set node gpu %q: %w", nodeID, err)
	}
	return nil
}

// GetNodeGPU returns the node's snapshot; ok is false when none has been
// reported yet.
func (db *DB) GetNodeGPU(ctx context.Context, nodeID string) (NodeGPU, bool, error) {
	row := db.QueryRowContext(ctx, `SELECT node_id, present, driver_version, runtime_installed, devices, updated_at FROM node_gpus WHERE node_id = ?`, nodeID)
	g, err := scanNodeGPU(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeGPU{}, false, nil
	}
	if err != nil {
		return NodeGPU{}, false, fmt.Errorf("store: get node gpu %q: %w", nodeID, err)
	}
	return g, true, nil
}

// ListNodeGPUs returns every stored snapshot keyed by node ID.
func (db *DB) ListNodeGPUs(ctx context.Context) (map[string]NodeGPU, error) {
	rows, err := db.QueryContext(ctx, `SELECT node_id, present, driver_version, runtime_installed, devices, updated_at FROM node_gpus`)
	if err != nil {
		return nil, fmt.Errorf("store: list node gpus: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]NodeGPU{}
	for rows.Next() {
		g, err := scanNodeGPU(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node gpu row: %w", err)
		}
		out[g.NodeID] = g
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate node gpu rows: %w", err)
	}
	return out, nil
}

func scanNodeGPU(scan func(dest ...any) error) (NodeGPU, error) {
	var (
		g               NodeGPU
		present, runtim int
		devices, at     string
	)
	if err := scan(&g.NodeID, &present, &g.Info.DriverVersion, &runtim, &devices, &at); err != nil {
		return NodeGPU{}, err
	}
	var devs []gpuDeviceJSON
	if err := json.Unmarshal([]byte(devices), &devs); err != nil {
		return NodeGPU{}, fmt.Errorf("parse devices: %w", err)
	}
	for _, d := range devs {
		g.Info.Devices = append(g.Info.Devices, gpu.Device{Index: d.Index, UUID: d.UUID, Name: d.Name,
			VRAMTotalMiB: d.VRAMTotalMiB, VRAMUsedMiB: d.VRAMUsedMiB, UtilizationPercent: d.Utilization})
	}
	g.Info.Present = present != 0
	g.Info.RuntimeInstalled = runtim != 0
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return NodeGPU{}, fmt.Errorf("parse updated_at: %w", err)
	}
	g.UpdatedAt = t
	return g, nil
}
