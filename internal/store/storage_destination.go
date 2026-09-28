package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// StorageOptions are the per-destination settings layered on a BackupTarget.
type StorageOptions struct {
	TargetID  string
	Preset    string
	PathStyle bool
	AccountID string
}

// SaveStorageOptions upserts the options row for a backup target.
func (db *DB) SaveStorageOptions(ctx context.Context, o StorageOptions) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO storage_destination_options (target_id, preset, path_style, account_id)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(target_id) DO UPDATE SET preset = excluded.preset,
			path_style = excluded.path_style, account_id = excluded.account_id
	`, o.TargetID, o.Preset, boolInt(o.PathStyle), o.AccountID)
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
		`SELECT preset, path_style, account_id FROM storage_destination_options WHERE target_id = ?`,
		targetID).Scan(&o.Preset, &pathStyle, &o.AccountID)
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
	rows, err := db.QueryContext(ctx, `SELECT target_id, preset, path_style, account_id FROM storage_destination_options`)
	if err != nil {
		return nil, fmt.Errorf("store: list storage options: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]StorageOptions{}
	for rows.Next() {
		var o StorageOptions
		var pathStyle int
		if err := rows.Scan(&o.TargetID, &o.Preset, &pathStyle, &o.AccountID); err != nil {
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
