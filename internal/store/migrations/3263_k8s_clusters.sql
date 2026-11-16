-- Connected Kubernetes clusters. Credentials live in the secrets store, never in this table.
CREATE TABLE IF NOT EXISTS k8s_clusters (
    name             TEXT PRIMARY KEY,
    server           TEXT NOT NULL,
    server_name      TEXT NOT NULL DEFAULT '',
    auth_mode        TEXT NOT NULL,
    ca_fingerprint   TEXT NOT NULL DEFAULT '',
    context_name     TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    last_checked_at  TEXT NOT NULL DEFAULT '',
    last_check_class TEXT NOT NULL DEFAULT ''
);
