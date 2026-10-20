-- Tombstone written before an app's desired state is deleted, removed only
-- once its containers are confirmed gone. Level-triggered reconcilers never
-- revisit a deleted service, so this is what makes delete retryable.
CREATE TABLE pending_teardowns (
    name            TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT NOT NULL DEFAULT '',
    last_attempt_at TEXT
);
