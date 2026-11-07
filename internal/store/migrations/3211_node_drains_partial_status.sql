-- A drain that moved some resources but left others blocked ends `partial`, not `failed`.
CREATE TABLE node_drains_new (
    id              TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL,
    target_node_id  TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'partial')),
    error           TEXT NOT NULL DEFAULT '',
    blocked         TEXT NOT NULL DEFAULT '[]',
    started_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    finished_at     TEXT NOT NULL DEFAULT ''
);
INSERT INTO node_drains_new (id, node_id, target_node_id, status, error, blocked, started_at, updated_at, finished_at)
    SELECT id, node_id, target_node_id, status, error, blocked, started_at, updated_at, finished_at FROM node_drains;
DROP TABLE node_drains; -- compat-break: rebuilt with every row to widen the status check
ALTER TABLE node_drains_new RENAME TO node_drains; -- compat-break: completes the rebuild above
CREATE UNIQUE INDEX idx_node_drains_active ON node_drains(node_id) WHERE status = 'running';
CREATE INDEX idx_node_drains_node ON node_drains(node_id, started_at DESC);
