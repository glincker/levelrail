-- Referrer hosts and coarse countries per app at one minute resolution.
-- Labels are capped at write time; only the referrer host and an ISO
-- country code are stored, never a URL, query string or client address.
CREATE TABLE request_audience (
    resource_id TEXT NOT NULL,
    kind        TEXT NOT NULL,
    label       TEXT NOT NULL,
    ts          INTEGER NOT NULL,
    requests    INTEGER NOT NULL,
    PRIMARY KEY (resource_id, kind, label, ts)
) WITHOUT ROWID;

CREATE INDEX idx_request_audience_ts ON request_audience (ts);
