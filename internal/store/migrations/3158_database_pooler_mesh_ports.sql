-- A pooled database used by an app on another node publishes the pooler on
-- the node's mesh address at this stable port, separate from the Postgres one.
CREATE TABLE database_pooler_mesh_ports (
    database_name TEXT PRIMARY KEY,
    port          INTEGER NOT NULL UNIQUE,
    created_at    TEXT NOT NULL
);
