-- Trust tier of a node that builds (server, managed or personal) and the per environment builder allow-list.
ALTER TABLE nodes ADD COLUMN build_trust TEXT NOT NULL DEFAULT 'server';
CREATE TABLE IF NOT EXISTS environment_builders (
    environment_id TEXT PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,
    allowed        TEXT NOT NULL DEFAULT '',
    updated_at     TEXT NOT NULL
);
