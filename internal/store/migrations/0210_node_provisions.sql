-- Cloud node provisioning: tracks a provider VM created on an operator's
-- behalf through to the join-token enrollment it's expected to complete.
-- A row's status is recomputed live against the provider API and the real
-- nodes table on every GET, not written by a background worker.
CREATE TABLE IF NOT EXISTS node_provisions (
    id                 TEXT PRIMARY KEY,
    provider           TEXT NOT NULL,
    region             TEXT NOT NULL,
    size               TEXT NOT NULL,
    name               TEXT NOT NULL,
    role               TEXT NOT NULL DEFAULT 'general',
    status             TEXT NOT NULL DEFAULT 'creating',
    provider_server_id TEXT NOT NULL DEFAULT '',
    ip_address         TEXT NOT NULL DEFAULT '',
    node_id            TEXT NOT NULL DEFAULT '',
    failure_reason     TEXT NOT NULL DEFAULT '',
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_node_provisions_status ON node_provisions (status);
