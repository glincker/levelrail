-- Operator-set node taints: a workload is kept off a tainted node unless it
-- carries a matching toleration.
CREATE TABLE node_taints (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL DEFAULT '',
    effect  TEXT NOT NULL CHECK (effect IN ('NoSchedule', 'PreferNoSchedule', 'NoExecute')),
    PRIMARY KEY (node_id, key, effect)
);
