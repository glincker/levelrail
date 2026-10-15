-- Deploy-time secret scanning. Findings hold a rule id, a location and a
-- short fingerprint hash, never the matched value.
CREATE TABLE app_secret_scan_settings (
    app_name   TEXT PRIMARY KEY,
    policy     TEXT NOT NULL DEFAULT 'warn',
    updated_at TEXT NOT NULL
);

CREATE TABLE secret_scan_allowlist (
    app_name    TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    reason      TEXT NOT NULL,
    actor       TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (app_name, fingerprint)
);

CREATE TABLE secret_scan_runs (
    attempt_id    TEXT PRIMARY KEY,
    app_name      TEXT NOT NULL,
    policy        TEXT NOT NULL,
    action        TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    error         TEXT NOT NULL DEFAULT '',
    files_scanned INTEGER NOT NULL DEFAULT 0,
    truncated     INTEGER NOT NULL DEFAULT 0,
    findings      TEXT NOT NULL DEFAULT '[]',
    scanned_at    TEXT NOT NULL
);

CREATE INDEX secret_scan_runs_app ON secret_scan_runs (app_name, scanned_at);
