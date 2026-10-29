package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// AgentEndpointScheme marks a backup target whose data lives in an enrolled node's confined directory (endpoint "agent://<node id>", bucket = directory).
const AgentEndpointScheme = "agent://"

// LocalEndpointScheme marks a backup target stored on a directory of the control plane host (endpoint "local://", bucket = path relative to APP_BACKUP_LOCAL_ROOT).
const LocalEndpointScheme = "local://"

// StorageOptions are the per-destination settings layered on a BackupTarget.
type StorageOptions struct {
	TargetID  string
	Preset    string
	PathStyle bool
	AccountID string
	// QuotaBytes caps what backups may keep on the destination; 0 means unlimited.
	QuotaBytes int64
}

// SaveStorageOptions upserts the options row for a backup target.
func (db *DB) SaveStorageOptions(ctx context.Context, o StorageOptions) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO storage_destination_options (target_id, preset, path_style, account_id, quota_bytes)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(target_id) DO UPDATE SET preset = excluded.preset,
			path_style = excluded.path_style, account_id = excluded.account_id, quota_bytes = excluded.quota_bytes
	`, o.TargetID, o.Preset, boolInt(o.PathStyle), o.AccountID, o.QuotaBytes)
	if err != nil {
		return fmt.Errorf("store: save storage options for %q: %w", o.TargetID, err)
	}
	return nil
}

// GetStorageOptions returns the options for a target. Targets created
// before this table existed report ok=false and callers derive defaults.
func (db *DB) GetStorageOptions(ctx context.Context, targetID string) (StorageOptions, bool, error) {
	o := StorageOptions{TargetID: targetID}
	var pathStyle int
	err := db.QueryRowContext(ctx,
		`SELECT preset, path_style, account_id, quota_bytes FROM storage_destination_options WHERE target_id = ?`,
		targetID).Scan(&o.Preset, &pathStyle, &o.AccountID, &o.QuotaBytes)
	if errors.Is(err, sql.ErrNoRows) {
		return StorageOptions{}, false, nil
	}
	if err != nil {
		return StorageOptions{}, false, fmt.Errorf("store: get storage options for %q: %w", targetID, err)
	}
	o.PathStyle = pathStyle != 0
	return o, true, nil
}

// ListStorageOptions returns every options row keyed by target ID.
func (db *DB) ListStorageOptions(ctx context.Context) (map[string]StorageOptions, error) {
	rows, err := db.QueryContext(ctx, `SELECT target_id, preset, path_style, account_id, quota_bytes FROM storage_destination_options`)
	if err != nil {
		return nil, fmt.Errorf("store: list storage options: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]StorageOptions{}
	for rows.Next() {
		var o StorageOptions
		var pathStyle int
		if err := rows.Scan(&o.TargetID, &o.Preset, &pathStyle, &o.AccountID, &o.QuotaBytes); err != nil {
			return nil, fmt.Errorf("store: scan storage options: %w", err)
		}
		o.PathStyle = pathStyle != 0
		out[o.TargetID] = o
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate storage options: %w", err)
	}
	return out, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// StorageDefault routes one data class to its default destination.
type StorageDefault struct {
	DataClass string
	TargetID  string
	UpdatedAt string
}

// SetStorageDefault points dataClass at targetID.
func (db *DB) SetStorageDefault(ctx context.Context, dataClass, targetID, updatedAt string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO storage_defaults (data_class, target_id, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(data_class) DO UPDATE SET target_id = excluded.target_id, updated_at = excluded.updated_at
	`, dataClass, targetID, updatedAt)
	if err != nil {
		return fmt.Errorf("store: set storage default for %q: %w", dataClass, err)
	}
	return nil
}

// ClearStorageDefault removes the default for dataClass.
func (db *DB) ClearStorageDefault(ctx context.Context, dataClass string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM storage_defaults WHERE data_class = ?`, dataClass); err != nil {
		return fmt.Errorf("store: clear storage default for %q: %w", dataClass, err)
	}
	return nil
}

// ListStorageDefaults returns every configured default.
func (db *DB) ListStorageDefaults(ctx context.Context) ([]StorageDefault, error) {
	rows, err := db.QueryContext(ctx, `SELECT data_class, target_id, updated_at FROM storage_defaults ORDER BY data_class`)
	if err != nil {
		return nil, fmt.Errorf("store: list storage defaults: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StorageDefault{}
	for rows.Next() {
		var d StorageDefault
		if err := rows.Scan(&d.DataClass, &d.TargetID, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan storage default: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate storage defaults: %w", err)
	}
	return out, nil
}

// StorageUsage is what succeeded backups currently occupy on a destination.
type StorageUsage struct {
	Objects int64
	Bytes   int64
}

// StorageUsageByTarget sums succeeded backup history per destination.
func (db *DB) StorageUsageByTarget(ctx context.Context) (map[string]StorageUsage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT target_id, COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM backup_history WHERE status = 'succeeded' GROUP BY target_id`)
	if err != nil {
		return nil, fmt.Errorf("store: storage usage: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]StorageUsage{}
	for rows.Next() {
		var id string
		var u StorageUsage
		if err := rows.Scan(&id, &u.Objects, &u.Bytes); err != nil {
			return nil, fmt.Errorf("store: scan storage usage: %w", err)
		}
		out[id] = u
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate storage usage: %w", err)
	}
	return out, nil
}
