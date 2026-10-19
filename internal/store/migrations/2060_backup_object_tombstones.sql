-- Objects whose row was already removed but whose bucket delete failed, retried by the backup scheduler sweep.
CREATE TABLE backup_object_tombstones (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id   TEXT NOT NULL,
    object_key  TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    attempts    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    UNIQUE (target_id, object_key)
);
