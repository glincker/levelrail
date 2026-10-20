-- Per-user console history. SQL text is the caller's own statement; results are never stored.
CREATE TABLE IF NOT EXISTS database_query_history (
    id             TEXT PRIMARY KEY,
    database_name  TEXT NOT NULL,
    principal_type TEXT NOT NULL,
    principal_id   TEXT NOT NULL,
    mode           TEXT NOT NULL,
    sql_text       TEXT NOT NULL,
    fingerprint    TEXT NOT NULL,
    ok             INTEGER NOT NULL,
    error          TEXT NOT NULL DEFAULT '',
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    row_count      INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_database_query_history_owner
    ON database_query_history (database_name, principal_type, principal_id, created_at DESC);
