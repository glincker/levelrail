-- Per-node and ordered workloads: a parent service owns hidden child services, one per
-- instance. Empty parent_service means an ordinary service, so existing rows are unchanged.
ALTER TABLE desired_services ADD COLUMN parent_service TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN instance_key TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_desired_services_parent ON desired_services(parent_service) WHERE parent_service != '';

-- One row per instance of a parent. generation is bumped on every move or force so a
-- returning node can tell its copy is stale; the fenced_* fields say why and where.
CREATE TABLE workload_instances (
    service        TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    instance_key   TEXT NOT NULL,
    child_service  TEXT NOT NULL,
    ordinal        INTEGER NOT NULL DEFAULT -1,
    node_id        TEXT NOT NULL DEFAULT '',
    state          TEXT NOT NULL DEFAULT 'pending'
                   CHECK (state IN ('pending', 'active', 'updating', 'fenced', 'retired')),
    generation     INTEGER NOT NULL DEFAULT 1 CHECK (generation >= 1),
    fenced_node_id TEXT NOT NULL DEFAULT '',
    fenced_reason  TEXT NOT NULL DEFAULT '',
    fenced_at      TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    PRIMARY KEY (service, instance_key)
);
CREATE UNIQUE INDEX idx_workload_instances_child ON workload_instances(child_service);
CREATE INDEX idx_workload_instances_node ON workload_instances(node_id) WHERE node_id != '';
