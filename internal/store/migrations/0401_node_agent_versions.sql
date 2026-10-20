CREATE TABLE IF NOT EXISTS node_agent_versions (
    seq          INTEGER PRIMARY KEY AUTOINCREMENT,
    id           TEXT NOT NULL UNIQUE,
    node_id      TEXT NOT NULL,
    node_name    TEXT NOT NULL DEFAULT '',
    from_version TEXT NOT NULL DEFAULT '',
    to_version   TEXT NOT NULL,
    observed_at  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS node_agent_versions_node ON node_agent_versions (node_id, seq);

CREATE TRIGGER IF NOT EXISTS node_agent_versions_no_update
BEFORE UPDATE ON node_agent_versions
BEGIN
    SELECT RAISE(ABORT, 'node agent version history is append-only');
END;
