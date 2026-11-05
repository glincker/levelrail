-- Hourly visitor rollups per app and domain. Only counts and hashed-visitor
-- totals live here, never a client address. epoch is the process start time:
-- visitor hashes use a per-process secret, so rows from different runs are
-- summed, never merged.
CREATE TABLE visitor_rollups (
    app        TEXT    NOT NULL,
    domain     TEXT    NOT NULL,
    hour_start INTEGER NOT NULL,
    epoch      INTEGER NOT NULL,
    requests   INTEGER NOT NULL DEFAULT 0,
    s2xx       INTEGER NOT NULL DEFAULT 0,
    s3xx       INTEGER NOT NULL DEFAULT 0,
    s4xx       INTEGER NOT NULL DEFAULT 0,
    s5xx       INTEGER NOT NULL DEFAULT 0,
    visitors   INTEGER NOT NULL DEFAULT 0,
    top_paths  TEXT    NOT NULL DEFAULT '[]',
    top_refs   TEXT    NOT NULL DEFAULT '[]',
    PRIMARY KEY (app, domain, hour_start, epoch)
);
CREATE INDEX visitor_rollups_hour ON visitor_rollups (hour_start);
