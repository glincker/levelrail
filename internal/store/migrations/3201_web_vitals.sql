-- Per-app web vitals settings: the public beacon key and the per-app enable switch.
CREATE TABLE app_vitals_config (
    app         TEXT PRIMARY KEY,
    public_key  TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL
);

-- Histogram rollup, no raw events: one counter per app, route, metric, hour, device class and fixed bucket.
CREATE TABLE web_vitals_rollup (
    app         TEXT NOT NULL,
    route       TEXT NOT NULL,
    metric      TEXT NOT NULL,
    hour        TEXT NOT NULL,
    device      TEXT NOT NULL,
    bucket      INTEGER NOT NULL,
    count       INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (app, route, metric, hour, device, bucket)
);
CREATE INDEX web_vitals_rollup_hour ON web_vitals_rollup (hour);
