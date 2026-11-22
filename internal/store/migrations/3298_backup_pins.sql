-- A keep-forever pin per backup: retention, pruning and delete all skip a pinned row. Kept apart from backup_history so its queries stay unchanged.
CREATE TABLE backup_pins (
    history_id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
