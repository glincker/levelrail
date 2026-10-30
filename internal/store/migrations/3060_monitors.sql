-- Uptime monitors: probe definitions, short-lived raw results and hourly rollups for the
-- experimental monitor engine (ADR 054). Nothing runs unless APP_MONITORS_ENABLED is on.
CREATE TABLE monitors (
    id          TEXT    PRIMARY KEY,
    name        TEXT    NOT NULL,
    kind        TEXT    NOT NULL CHECK (kind IN ('http', 'tcp')),
    interval_s  INTEGER NOT NULL,
    timeout_ms  INTEGER NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    spec        TEXT    NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE TABLE monitor_results (
    monitor_id  TEXT    NOT NULL,
    at          INTEGER NOT NULL,
    location    TEXT    NOT NULL DEFAULT 'cp',
    ok          INTEGER NOT NULL,
    latency_ms  INTEGER NOT NULL DEFAULT 0,
    code        INTEGER NOT NULL DEFAULT 0,
    reason      TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (monitor_id, at, location)
) WITHOUT ROWID;

CREATE TABLE monitor_rollup_1h (
    monitor_id  TEXT    NOT NULL,
    bucket      INTEGER NOT NULL,
    total       INTEGER NOT NULL,
    failed      INTEGER NOT NULL,
    lat_sum     INTEGER NOT NULL,
    lat_max     INTEGER NOT NULL,
    PRIMARY KEY (monitor_id, bucket)
) WITHOUT ROWID;
