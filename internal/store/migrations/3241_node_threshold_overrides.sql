-- Per-node alert threshold overrides. A NULL column means the node uses the
-- platform default runtime setting for that resource.
CREATE TABLE node_threshold_overrides (
    node_id        TEXT PRIMARY KEY,
    cpu_percent    INTEGER,
    memory_percent INTEGER,
    disk_percent   INTEGER,
    updated_at     TEXT NOT NULL DEFAULT ''
);
