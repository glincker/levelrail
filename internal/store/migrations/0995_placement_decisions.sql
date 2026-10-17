CREATE TABLE IF NOT EXISTS placement_decisions (
    kind       TEXT NOT NULL,
    resource   TEXT NOT NULL,
    trigger    TEXT NOT NULL,
    node_id    TEXT NOT NULL,
    decided_at TEXT NOT NULL,
    body       TEXT NOT NULL,
    PRIMARY KEY (kind, resource)
);
