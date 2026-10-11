-- Per-route and per-status request counters at one minute resolution, so a
-- latency spike can be explained by which routes and status codes drove it.
-- key is a normalized route template for kind 'route' and a status code for
-- kind 'status'. Cardinality is capped at write time.
CREATE TABLE request_breakdown (
    resource_id    TEXT NOT NULL,
    kind           TEXT NOT NULL,
    key            TEXT NOT NULL,
    ts             INTEGER NOT NULL,
    requests       INTEGER NOT NULL,
    errors_4xx     INTEGER NOT NULL,
    errors_5xx     INTEGER NOT NULL,
    latency_ms_sum REAL NOT NULL,
    PRIMARY KEY (resource_id, kind, key, ts)
) WITHOUT ROWID;

CREATE INDEX idx_request_breakdown_ts ON request_breakdown (ts);
