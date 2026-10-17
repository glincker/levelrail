-- Per-upstream (replica) latency histograms at one minute resolution and a
-- bounded sample of slow or failed requests. Upstream labels and routes are
-- capped at write time; raw URLs, query strings and client addresses are
-- never stored.
CREATE TABLE request_upstreams (
    resource_id     TEXT NOT NULL,
    upstream        TEXT NOT NULL,
    ts              INTEGER NOT NULL,
    requests        INTEGER NOT NULL,
    errors_4xx      INTEGER NOT NULL,
    errors_5xx      INTEGER NOT NULL,
    upstream_errors INTEGER NOT NULL,
    b0 INTEGER NOT NULL, b1 INTEGER NOT NULL, b2 INTEGER NOT NULL, b3 INTEGER NOT NULL,
    b4 INTEGER NOT NULL, b5 INTEGER NOT NULL, b6 INTEGER NOT NULL, b7 INTEGER NOT NULL,
    b8 INTEGER NOT NULL, b9 INTEGER NOT NULL, b10 INTEGER NOT NULL, b11 INTEGER NOT NULL,
    PRIMARY KEY (resource_id, upstream, ts)
) WITHOUT ROWID;

CREATE INDEX idx_request_upstreams_ts ON request_upstreams (ts);

CREATE TABLE request_samples (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    resource_id TEXT NOT NULL,
    ts          INTEGER NOT NULL,
    route       TEXT NOT NULL,
    method      TEXT NOT NULL,
    status      INTEGER NOT NULL,
    duration_ms REAL NOT NULL,
    upstream    TEXT NOT NULL,
    request_id  TEXT NOT NULL
);

CREATE INDEX idx_request_samples_resource_ts ON request_samples (resource_id, ts);
