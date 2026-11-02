-- A database name can be reused after a delete, so backups are tied to the
-- database instance that took them. Rows written before this migration keep an
-- empty instance_id and are matched by creation time instead.
ALTER TABLE desired_databases ADD COLUMN instance_id TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
ALTER TABLE backup_history ADD COLUMN instance_id TEXT NOT NULL DEFAULT '';
ALTER TABLE base_backup_history ADD COLUMN instance_id TEXT NOT NULL DEFAULT '';

-- Existing databases predate their oldest surviving backup, so their creation
-- time is the earlier of that backup and the last update.
UPDATE desired_databases SET
    instance_id = lower(hex(randomblob(8))),
    created_at = MIN(
        updated_at,
        COALESCE((SELECT MIN(started_at) FROM backup_history WHERE backup_history.database_name = desired_databases.name), updated_at),
        COALESCE((SELECT MIN(started_at) FROM base_backup_history WHERE base_backup_history.database_name = desired_databases.name), updated_at)
    );
