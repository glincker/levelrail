CREATE TABLE IF NOT EXISTS fixit_attempts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    fix_id      TEXT NOT NULL,
    subject     TEXT NOT NULL,
    at          TEXT NOT NULL,
    actor       TEXT NOT NULL DEFAULT '',
    outcome     TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_fixit_attempts_fix ON fixit_attempts (fix_id, subject, id);

CREATE TABLE IF NOT EXISTS fixit_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);
