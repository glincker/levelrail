-- Metadata about a connected cloud provider (or a manual data center).
-- The credential itself stays in the secrets store; nothing secret is here.

CREATE TABLE node_provider_connections (
    provider         TEXT PRIMARY KEY,
    method           TEXT NOT NULL,
    label            TEXT NOT NULL DEFAULT '',
    location         TEXT NOT NULL DEFAULT '',
    notes            TEXT NOT NULL DEFAULT '',
    connected_by     TEXT NOT NULL DEFAULT '',
    connected_at     TEXT NOT NULL,
    last_tested_at   TEXT,
    last_test_ok     INTEGER NOT NULL DEFAULT 0,
    last_test_detail TEXT NOT NULL DEFAULT ''
);
