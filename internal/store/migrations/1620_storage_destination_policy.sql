ALTER TABLE storage_destination_options ADD COLUMN quota_bytes INTEGER NOT NULL DEFAULT 0;

CREATE TABLE storage_defaults (
    data_class TEXT PRIMARY KEY,
    target_id  TEXT NOT NULL REFERENCES backup_targets(id) ON DELETE CASCADE,
    updated_at TEXT NOT NULL
);
