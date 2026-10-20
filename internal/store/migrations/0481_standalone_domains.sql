-- Domains not owned by any app that proxy to an operator-chosen target: a
-- local port, an address, a URL, another app's port or a container. The
-- domain column is shared with service_domains and static_site_domains:
-- store.domainOwner checks all three so a hostname has one owner.
CREATE TABLE standalone_domains (
    domain               TEXT PRIMARY KEY,
    target               TEXT NOT NULL,
    target_kind          TEXT NOT NULL,
    upstream_tls         INTEGER NOT NULL DEFAULT 0,
    tls_skip_verify      INTEGER NOT NULL DEFAULT 0,
    tls_server_name      TEXT NOT NULL DEFAULT '',
    host_header          TEXT NOT NULL DEFAULT '',
    websocket            INTEGER NOT NULL DEFAULT 1,
    dial_timeout_seconds INTEGER NOT NULL DEFAULT 0,
    read_timeout_seconds INTEGER NOT NULL DEFAULT 0,
    strip_prefix         TEXT NOT NULL DEFAULT '',
    description          TEXT NOT NULL DEFAULT '',
    created_by           TEXT NOT NULL DEFAULT '',
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);
