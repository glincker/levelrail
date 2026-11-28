package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetAppVolumeInstance returns the app instance recorded for a volume, or "" when none was.
func (db *DB) GetAppVolumeInstance(ctx context.Context, volume string) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT instance_id FROM app_volume_owners WHERE volume_name = ?`, volume).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: read owner of volume %q: %w", volume, err)
	}
	return id, nil
}

// SetAppVolumeInstance records or replaces the app instance that owns a volume.
func (db *DB) SetAppVolumeInstance(ctx context.Context, volume, instanceID string) error {
	if _, err := db.ExecContext(ctx, `
		INSERT INTO app_volume_owners (volume_name, instance_id) VALUES (?, ?)
		ON CONFLICT (volume_name) DO UPDATE SET instance_id = excluded.instance_id
	`, volume, instanceID); err != nil {
		return fmt.Errorf("store: record owner of volume %q: %w", volume, err)
	}
	return nil
}

// ListAppVolumeInstances returns every recorded volume owner keyed by volume name.
func (db *DB) ListAppVolumeInstances(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT volume_name, instance_id FROM app_volume_owners`)
	if err != nil {
		return nil, fmt.Errorf("store: list volume owners: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var v, id string
		if err := rows.Scan(&v, &id); err != nil {
			return nil, fmt.Errorf("store: scan volume owner: %w", err)
		}
		out[v] = id
	}
	return out, rows.Err()
}
