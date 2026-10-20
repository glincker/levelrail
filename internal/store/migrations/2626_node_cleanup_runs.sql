-- History of executed per-node cleanups (manual and auto), bytes freed per class.
CREATE TABLE node_cleanup_runs (
    id              TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    trigger         TEXT NOT NULL,
    actor           TEXT NOT NULL DEFAULT '',
    reclaimed_bytes INTEGER NOT NULL DEFAULT 0,
    body            TEXT NOT NULL,
    error           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX node_cleanup_runs_node ON node_cleanup_runs (node_id, started_at DESC);
