package store

import (
	"context"
	"fmt"
)

// BackupUsage is the stored size of one resource's succeeded backups.
type BackupUsage struct {
	ResourceKind string
	DatabaseName string
	ServiceName  string
	VolumeName   string
	Count        int64
	Bytes        int64
}

// SumBackupUsage totals succeeded backup bytes per database or app volume.
func (db *DB) SumBackupUsage(ctx context.Context) ([]BackupUsage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT resource_kind, database_name, service_name, volume_name, COUNT(*), COALESCE(SUM(size_bytes), 0)
		FROM backup_history
		WHERE status = ?
		GROUP BY resource_kind, database_name, service_name, volume_name
	`, BackupStatusSucceeded)
	if err != nil {
		return nil, fmt.Errorf("store: sum backup usage: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []BackupUsage
	for rows.Next() {
		var u BackupUsage
		if err := rows.Scan(&u.ResourceKind, &u.DatabaseName, &u.ServiceName, &u.VolumeName, &u.Count, &u.Bytes); err != nil {
			return nil, fmt.Errorf("store: scan backup usage: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate backup usage: %w", err)
	}
	return out, nil
}
