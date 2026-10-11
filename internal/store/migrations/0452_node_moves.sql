-- node_moves is the persisted state machine behind "move this app or
-- database to another node": a runner advances phase one step at a time,
-- so a control plane restart resumes a move instead of stranding it.
CREATE TABLE node_moves (
    id                  TEXT PRIMARY KEY,
    kind                TEXT NOT NULL CHECK (kind IN ('app', 'database')),
    resource            TEXT NOT NULL,
    from_node_id        TEXT NOT NULL,
    to_node_id          TEXT NOT NULL,
    stateful            INTEGER NOT NULL DEFAULT 0,
    volumes             TEXT NOT NULL DEFAULT '[]',
    phase               TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'rolled_back')),
    error               TEXT NOT NULL DEFAULT '',
    attempts            INTEGER NOT NULL DEFAULT 0,
    steps               TEXT NOT NULL DEFAULT '[]',
    drain_id            TEXT NOT NULL DEFAULT '',
    placed_at           TEXT NOT NULL DEFAULT '',
    verify_deadline     TEXT NOT NULL DEFAULT '',
    started_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    finished_at         TEXT NOT NULL DEFAULT ''
);

-- At most one in-flight move per resource: the partial unique index is the lock.
CREATE UNIQUE INDEX idx_node_moves_active ON node_moves(kind, resource) WHERE status = 'running';
CREATE INDEX idx_node_moves_resource ON node_moves(kind, resource, started_at DESC);
CREATE INDEX idx_node_moves_drain ON node_moves(drain_id) WHERE drain_id <> '';

CREATE TABLE node_drains (
    id              TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL,
    target_node_id  TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    error           TEXT NOT NULL DEFAULT '',
    blocked         TEXT NOT NULL DEFAULT '[]',
    started_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    finished_at     TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_node_drains_active ON node_drains(node_id) WHERE status = 'running';
CREATE INDEX idx_node_drains_node ON node_drains(node_id, started_at DESC);
