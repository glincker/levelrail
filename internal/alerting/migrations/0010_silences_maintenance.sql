-- Silences keep alerts evaluating and recorded but stop them notifying.
-- matchers_json is the SilenceMatcher shape; a row is never deleted so
-- expired silences stay in history (expired_at marks an early expiry).
CREATE TABLE alert_silences (
    id            TEXT PRIMARY KEY,
    matchers_json TEXT NOT NULL,
    starts_at     TEXT NOT NULL,
    ends_at       TEXT NOT NULL,
    created_by    TEXT NOT NULL DEFAULT '',
    reason        TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    expired_at    TEXT
);

CREATE INDEX idx_alert_silences_ends ON alert_silences (ends_at);

-- Maintenance windows are recurring silences: a cron start, a duration
-- and an IANA timezone, scoped to all alerts, a set of apps or a set of nodes.
CREATE TABLE alert_maintenance_windows (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    cron             TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    scope            TEXT NOT NULL DEFAULT 'all',
    targets_json     TEXT NOT NULL DEFAULT '[]',
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
