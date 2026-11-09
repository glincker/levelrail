-- App expiry for temporary environments: at expires_at the app is stopped,
-- and only after a grace period deleted, unless something marks it kept.
-- incarnation pins the row to one life of the app name.
CREATE TABLE app_expiry (
    app_name TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL,
    delete_data INTEGER NOT NULL DEFAULT 0,
    set_by TEXT NOT NULL DEFAULT '',
    set_at TEXT NOT NULL,
    notified_at TEXT NOT NULL DEFAULT '',
    stopped_at TEXT NOT NULL DEFAULT '',
    kept_reason TEXT NOT NULL DEFAULT '',
    incarnation TEXT NOT NULL DEFAULT '',
    setter_type TEXT NOT NULL DEFAULT '',
    setter_id TEXT NOT NULL DEFAULT '',
    stopped_by TEXT NOT NULL DEFAULT '',
    claimed_at TEXT NOT NULL DEFAULT ''
);
