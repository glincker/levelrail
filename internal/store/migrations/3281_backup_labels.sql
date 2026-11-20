-- A short operator label per backup, kept apart from backup_history so every history query stays unchanged.
CREATE TABLE backup_labels (
    history_id TEXT PRIMARY KEY,
    label      TEXT NOT NULL,
    created_at TEXT NOT NULL
);
