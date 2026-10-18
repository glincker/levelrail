-- Staged canaries: stage schedule and breach thresholds live on the release
-- row, a per-app policy comes from app.yaml, and the last outcome is kept
-- after the row is gone so the dashboard can say why a canary rolled back.
ALTER TABLE canary_releases ADD COLUMN stages TEXT NOT NULL DEFAULT '';
ALTER TABLE canary_releases ADD COLUMN stage_index INTEGER NOT NULL DEFAULT 0;
ALTER TABLE canary_releases ADD COLUMN interval_secs INTEGER NOT NULL DEFAULT 0;
ALTER TABLE canary_releases ADD COLUMN max_error_rate REAL NOT NULL DEFAULT 0;
ALTER TABLE canary_releases ADD COLUMN max_p95_ratio REAL NOT NULL DEFAULT 0;
ALTER TABLE canary_releases ADD COLUMN status_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS canary_policies (
    service_name   TEXT PRIMARY KEY,
    stages         TEXT NOT NULL,
    interval_secs  INTEGER NOT NULL,
    max_error_rate REAL NOT NULL,
    max_p95_ratio  REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS canary_outcomes (
    service_name TEXT PRIMARY KEY,
    outcome      TEXT NOT NULL,
    reason       TEXT NOT NULL,
    image        TEXT NOT NULL,
    at           TEXT NOT NULL
);
