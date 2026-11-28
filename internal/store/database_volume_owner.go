package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// DatabaseVolumeOwner is the team and project that own the data volume of the
// database called Name, whether or not that database still exists.
type DatabaseVolumeOwner struct {
	Name      string
	TeamID    string
	ProjectID string
}

// GetDatabaseVolumeOwner returns the recorded owner, or nil when none was recorded.
func (db *DB) GetDatabaseVolumeOwner(ctx context.Context, name string) (*DatabaseVolumeOwner, error) {
	o := DatabaseVolumeOwner{Name: name}
	err := db.QueryRowContext(ctx, `SELECT team_id, project_id FROM database_volume_owners WHERE name = ?`, name).Scan(&o.TeamID, &o.ProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get database volume owner %q: %w", name, err)
	}
	return &o, nil
}

// SetDatabaseVolumeOwner records or replaces the owner of a database volume.
func (db *DB) SetDatabaseVolumeOwner(ctx context.Context, o DatabaseVolumeOwner) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO database_volume_owners (name, team_id, project_id, recorded_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET team_id = excluded.team_id, project_id = excluded.project_id, recorded_at = excluded.recorded_at
	`, o.Name, o.TeamID, o.ProjectID, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: set database volume owner %q: %w", o.Name, err)
	}
	return nil
}

// DeleteDatabaseVolumeOwner forgets the owner, which makes the volume unowned.
func (db *DB) DeleteDatabaseVolumeOwner(ctx context.Context, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM database_volume_owners WHERE name = ?`, name); err != nil {
		return fmt.Errorf("store: delete database volume owner %q: %w", name, err)
	}
	return nil
}
