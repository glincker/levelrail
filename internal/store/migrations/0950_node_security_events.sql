-- Bounded per-node security event log and the agent-reported trust state
-- behind the node trust panel. Times are unix milliseconds. node_id is ''
-- for events that cannot be tied to a known node (unknown subject).
CREATE TABLE node_security_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id    TEXT NOT NULL DEFAULT '',
    kind       TEXT NOT NULL,
    outcome    TEXT NOT NULL,
    reason     TEXT NOT NULL DEFAULT '',
    source_ip  TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '{}',
    count      INTEGER NOT NULL DEFAULT 1,
    first_at   INTEGER NOT NULL,
    last_at    INTEGER NOT NULL
);
CREATE INDEX idx_node_security_events_node ON node_security_events(node_id, last_at DESC);

CREATE TABLE node_trust_state (
    node_id                  TEXT PRIMARY KEY,
    ca_pin_state             TEXT NOT NULL DEFAULT '',
    ca_pin_at                INTEGER,
    renew_next_at            INTEGER,
    renew_last_error         TEXT NOT NULL DEFAULT '',
    renew_failed_at          INTEGER,
    renew_next_retry_at      INTEGER,
    renew_consecutive_fails  INTEGER NOT NULL DEFAULT 0,
    renew_reported_at        INTEGER
);
