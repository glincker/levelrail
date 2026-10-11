-- A managed database used by an app on another node is published on its
-- node's mesh address at this stable port, so the connection URL survives
-- the database or the app moving between nodes.
CREATE TABLE database_mesh_ports (
    database_name TEXT PRIMARY KEY,
    port          INTEGER NOT NULL UNIQUE,
    created_at    TEXT NOT NULL
);
