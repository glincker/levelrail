-- Per-app rolling deploy settings from app.yaml's rollout: block. Empty
-- strings inherit the APP_ROLLOUT_* env defaults.
CREATE TABLE IF NOT EXISTS rollout_policies (
    service_name    TEXT PRIMARY KEY,
    max_surge       TEXT NOT NULL DEFAULT '',
    max_unavailable TEXT NOT NULL DEFAULT '',
    min_available   TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL DEFAULT ''
);
