-- Per-app opt-in for redeploying when a registry tag moves to a new digest,
-- checked by internal/imageupdate.Scheduler. Kept apart from services so the
-- desired-state row (and its positional INSERT) stays untouched.
CREATE TABLE IF NOT EXISTS image_auto_updates (
    service_name    TEXT PRIMARY KEY,
    enabled         INTEGER NOT NULL DEFAULT 0,
    last_checked_at TEXT,
    last_result     TEXT NOT NULL DEFAULT '',
    webhook_hash    TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL
);
