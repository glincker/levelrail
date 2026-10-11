-- Operator-set node labels, and the per-app or per-database selectors whose
-- every key=value must match a node's labels before the scheduler places there.
CREATE TABLE node_labels (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (node_id, key)
);

CREATE TABLE placement_selectors (
    kind     TEXT NOT NULL CHECK (kind IN ('app', 'database')),
    resource TEXT NOT NULL,
    key      TEXT NOT NULL,
    value    TEXT NOT NULL,
    PRIMARY KEY (kind, resource, key)
);
