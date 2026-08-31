package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrServiceVolumeBackupNotFound is returned by GetServiceVolumeBackupSchedule
// when no row exists for the given service/volume pair.
var ErrServiceVolumeBackupNotFound = errors.New("store: service volume backup schedule not found")

// ServiceVolumeBackupConfig is one service volume's scheduled-backup
// config: migrations/0075_service_volume_backups.sql's own row, the
// volume counterpart of DesiredDatabase's BackupTargetID/BackupSchedule/
// BackupRetain/BackupRetainDays fields. ServiceName/VolumeName identify
// the volume the same way internal/api's resolveServiceVolume does:
// VolumeName is the volume's logical name (spec.Volume.Name), not the
// resolved Docker volume name internal/deploy's volumeName() computes.
type ServiceVolumeBackupConfig struct {
	ServiceName      string
	VolumeName       string
	BackupTargetID   string
	BackupSchedule   string
	BackupRetain     int
	BackupRetainDays int
}

// SetServiceVolumeBackupSchedule upserts serviceName/volumeName's backup
// schedule config, the volume counterpart of SetDatabaseBackupSchedule
// (database.go). Passing targetID="" and schedule="" clears it back to
// "not scheduled", the same "" sentinel SetDatabaseBackupSchedule already
// establishes: internal/backup.Scheduler's own ListScheduledServiceVolumes
// query treats an empty schedule exactly like the row was never written.
func (db *DB) SetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName, targetID, schedule string, retain, retainDays int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO service_volume_backups (service_name, volume_name, backup_target_id, backup_schedule, backup_retain, backup_retain_days)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, volume_name) DO UPDATE SET
			backup_target_id = excluded.backup_target_id,
			backup_schedule = excluded.backup_schedule,
			backup_retain = excluded.backup_retain,
			backup_retain_days = excluded.backup_retain_days
	`, serviceName, volumeName, sql.NullString{String: targetID, Valid: targetID != ""}, schedule, retain, retainDays)
	if err != nil {
		return fmt.Errorf("store: set backup schedule for service %q volume %q: %w", serviceName, volumeName, err)
	}
	return nil
}

// GetServiceVolumeBackupSchedule returns serviceName/volumeName's backup
// schedule config, or ErrServiceVolumeBackupNotFound if it was never set.
func (db *DB) GetServiceVolumeBackupSchedule(ctx context.Context, serviceName, volumeName string) (ServiceVolumeBackupConfig, error) {
	var (
		c              ServiceVolumeBackupConfig
		backupTargetID sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT service_name, volume_name, backup_target_id, backup_schedule, backup_retain, backup_retain_days
		FROM service_volume_backups
		WHERE service_name = ? AND volume_name = ?
	`, serviceName, volumeName).Scan(&c.ServiceName, &c.VolumeName, &backupTargetID, &c.BackupSchedule, &c.BackupRetain, &c.BackupRetainDays)
	if errors.Is(err, sql.ErrNoRows) {
		return ServiceVolumeBackupConfig{}, ErrServiceVolumeBackupNotFound
	}
	if err != nil {
		return ServiceVolumeBackupConfig{}, fmt.Errorf("store: get backup schedule for service %q volume %q: %w", serviceName, volumeName, err)
	}
	c.BackupTargetID = backupTargetID.String
	return c, nil
}

// ListScheduledServiceVolumes returns every service volume with both a
// non-empty backup_schedule and a resolved backup_target_id, ordered by
// service_name then volume_name, the volume counterpart of
// ListScheduledDatabases (database.go): see that method's own doc
// comment for why both are required together rather than schedule alone.
func (db *DB) ListScheduledServiceVolumes(ctx context.Context) ([]ServiceVolumeBackupConfig, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, volume_name, backup_target_id, backup_schedule, backup_retain, backup_retain_days
		FROM service_volume_backups
		WHERE backup_schedule != '' AND backup_target_id IS NOT NULL
		ORDER BY service_name, volume_name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list scheduled service volumes: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []ServiceVolumeBackupConfig
	for rows.Next() {
		var (
			c              ServiceVolumeBackupConfig
			backupTargetID sql.NullString
		)
		if err := rows.Scan(&c.ServiceName, &c.VolumeName, &backupTargetID, &c.BackupSchedule, &c.BackupRetain, &c.BackupRetainDays); err != nil {
			return nil, fmt.Errorf("store: scan scheduled service volume row: %w", err)
		}
		c.BackupTargetID = backupTargetID.String
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate scheduled service volume rows: %w", err)
	}
	return out, nil
}
