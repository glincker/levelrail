CREATE TABLE app_autoscale (
    app_name                   TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    min_replicas               INTEGER NOT NULL CHECK (min_replicas >= 1),
    max_replicas               INTEGER NOT NULL CHECK (max_replicas >= min_replicas),
    metric                     TEXT NOT NULL CHECK (metric IN ('cpu_percent', 'memory_percent', 'requests_per_replica')),
    target                     REAL NOT NULL CHECK (target > 0),
    scale_up_tolerance_pct     INTEGER NOT NULL DEFAULT 10,
    scale_down_tolerance_pct   INTEGER NOT NULL DEFAULT 30,
    scale_up_cooldown_secs     INTEGER NOT NULL DEFAULT 60,
    scale_down_cooldown_secs   INTEGER NOT NULL DEFAULT 600,
    stabilization_secs         INTEGER NOT NULL DEFAULT 300,
    max_step_up                INTEGER NOT NULL DEFAULT 4,
    max_step_down              INTEGER NOT NULL DEFAULT 1,
    updated_at                 TEXT NOT NULL
);

CREATE TABLE app_autoscale_state (
    app_name        TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    current         INTEGER NOT NULL DEFAULT 0,
    desired         INTEGER NOT NULL DEFAULT 0,
    value           REAL NOT NULL DEFAULT 0,
    value_known     INTEGER NOT NULL DEFAULT 0,
    reason          TEXT NOT NULL DEFAULT '',
    blocked_by      TEXT NOT NULL DEFAULT '',
    message         TEXT NOT NULL DEFAULT '',
    last_scale_up   TEXT NOT NULL DEFAULT '',
    last_scale_down TEXT NOT NULL DEFAULT '',
    down_since      TEXT NOT NULL DEFAULT '',
    down_floor      INTEGER NOT NULL DEFAULT 0,
    updated_at      TEXT NOT NULL
);

CREATE TABLE app_autoscale_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name   TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    current    INTEGER NOT NULL DEFAULT 0,
    desired    INTEGER NOT NULL DEFAULT 0,
    value      REAL NOT NULL DEFAULT 0,
    reason     TEXT NOT NULL DEFAULT '',
    blocked_by TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    at         TEXT NOT NULL
);
CREATE INDEX idx_app_autoscale_events_app_at ON app_autoscale_events (app_name, at);
