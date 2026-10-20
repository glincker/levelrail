-- One row per node status transition (online, offline, cordoned),
-- written by UpdateNodeStatus only when the status actually changes.
-- Capped per node in application code, not here.
CREATE TABLE node_status_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_node_status_events_node ON node_status_events (node_id, id DESC);
