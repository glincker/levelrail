-- What Levelrail applied to a connected cluster. Env values are never stored here.
CREATE TABLE IF NOT EXISTS k8s_deployments (
    cluster_name TEXT NOT NULL,
    app          TEXT NOT NULL,
    namespace    TEXT NOT NULL,
    image        TEXT NOT NULL,
    port         INTEGER NOT NULL,
    replicas     INTEGER NOT NULL,
    host         TEXT NOT NULL DEFAULT '',
    allow_root   INTEGER NOT NULL DEFAULT 0,
    objects      TEXT NOT NULL DEFAULT '[]',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (cluster_name, app)
);
