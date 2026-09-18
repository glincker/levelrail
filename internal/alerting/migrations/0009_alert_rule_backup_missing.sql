-- kind='backup_missing' rules watch one database's or one service
-- volume's scheduled backup cadence (store.DesiredDatabase.
-- BackupSchedule or store.ServiceVolumeBackupConfig.BackupSchedule),
-- firing once its last successful run falls further behind that
-- schedule's expected interval than for_duration_seconds allows (reused
-- as the grace period, see internal/alerting/backup_missing.go).
-- resource_id stays the owning app's resource id, same as every other
-- app-scoped rule kind; these four columns pick out which database or
-- service volume the rule actually watches.
ALTER TABLE alert_rules ADD COLUMN backup_resource_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN backup_database_name TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN backup_service_name TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN backup_volume_name TEXT NOT NULL DEFAULT '';
