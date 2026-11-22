-- Node pools name a label selector apps can target; app_warm lists where an app's image is kept pulled.
CREATE TABLE node_pools (
    name TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
CREATE TABLE node_pool_labels (
    pool TEXT NOT NULL REFERENCES node_pools(name) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (pool, key)
);
CREATE TABLE app_pools (
    app TEXT PRIMARY KEY,
    pool TEXT NOT NULL REFERENCES node_pools(name)
);
CREATE TABLE app_warm (
    app TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('node', 'label')),
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (app, kind, key, value)
);
