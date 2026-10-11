-- Last probe of a backup target's bucket protection: object lock and
-- versioning decide whether the platform's own key can silently delete or
-- overwrite a backup.
CREATE TABLE backup_target_protection (
    target_id    TEXT PRIMARY KEY,
    object_lock  INTEGER NOT NULL DEFAULT 0,
    lock_mode    TEXT NOT NULL DEFAULT '',
    versioning   TEXT NOT NULL DEFAULT '',
    can_delete   INTEGER NOT NULL DEFAULT 0,
    probe_error  TEXT NOT NULL DEFAULT '',
    checked_at   TEXT NOT NULL
);
