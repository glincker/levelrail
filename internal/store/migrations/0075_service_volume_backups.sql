-- App-level persistent volumes (spec.Service.Volumes, migrations/0041)
-- had zero backup coverage: backup_history/restore_history were built
-- database-only. This extends both tables to a second resource kind
-- rather than duplicating Runner/Scheduler/history into a parallel
-- system, per precedent already set by this schema (a new column, not a
-- new table, when an existing row shape mostly already fits:
-- migrations/0023's own comment gives the identical reasoning for
-- desired_databases' backup_target_id/schedule/retain columns).
--
-- resource_kind discriminates the two; service_name/volume_name are the
-- volume identity, populated only when resource_kind = 'volume' and left
-- '' for every existing (and every future database) row, the same
-- "empty string is the real, meaningful zero value" convention
-- backup_schedule itself already uses (migrations/0023). database_name
-- stays '' on a volume row: the two identity shapes never overlap, so
-- every existing query keyed on database_name (ListBackupHistory,
-- PruneBackupHistory, ...) keeps matching only real database rows
-- without needing its own resource_kind filter added.
ALTER TABLE backup_history ADD COLUMN resource_kind TEXT NOT NULL DEFAULT 'database' CHECK (resource_kind IN ('database', 'volume'));
ALTER TABLE backup_history ADD COLUMN service_name TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_history ADD COLUMN volume_name TEXT NOT NULL DEFAULT '';

ALTER TABLE restore_history ADD COLUMN resource_kind TEXT NOT NULL DEFAULT 'database' CHECK (resource_kind IN ('database', 'volume'));
ALTER TABLE restore_history ADD COLUMN service_name TEXT NOT NULL DEFAULT '';
ALTER TABLE restore_history ADD COLUMN volume_name TEXT NOT NULL DEFAULT '';

-- Supports ListServiceVolumeBackupHistory/ListServiceVolumeRestoreHistory's
-- own query shape, mirroring idx_backup_history_database_name /
-- idx_restore_history_database_name (migrations/0018/0019) for the
-- volume identity instead of a database name.
CREATE INDEX idx_backup_history_service_volume ON backup_history(service_name, volume_name, started_at DESC) WHERE resource_kind = 'volume';
CREATE INDEX idx_restore_history_service_volume ON restore_history(service_name, volume_name, started_at DESC) WHERE resource_kind = 'volume';

-- service_volume_backups is desired_databases' backup_target_id/
-- backup_schedule/backup_retain/backup_retain_days columns (0023), moved
-- into its own table rather than more columns on desired_services: a
-- volume's backup schedule is keyed on (service_name, volume_name), not
-- one-to-one with a service row the way a database's schedule is
-- one-to-one with a desired_databases row, and store.ServiceVolume
-- itself already lives inside desired_services as an opaque JSON blob
-- (service.go), not queryable columns a WHERE clause could filter on.
CREATE TABLE service_volume_backups (
    service_name       TEXT NOT NULL,
    volume_name        TEXT NOT NULL,
    backup_target_id   TEXT REFERENCES backup_targets(id) ON DELETE SET NULL,
    backup_schedule    TEXT NOT NULL DEFAULT '',
    backup_retain      INTEGER NOT NULL DEFAULT 0,
    backup_retain_days INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (service_name, volume_name)
);

-- Supports ListScheduledServiceVolumes, evaluated once per
-- internal/backup.Scheduler tick, the same reasoning
-- idx_desired_databases_backup_schedule (0023) gives for the database
-- side of the identical scheduler.
CREATE INDEX idx_service_volume_backups_schedule ON service_volume_backups (backup_schedule) WHERE backup_schedule != '';
