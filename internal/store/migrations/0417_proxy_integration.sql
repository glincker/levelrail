-- Managed routes in an existing reverse proxy that owns ports 80 and 443.
-- Empty override columns mean "use what detection finds".
CREATE TABLE proxy_integration_settings (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    mode             TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'traefik_file')),
    dynamic_dir      TEXT NOT NULL DEFAULT '',
    entrypoint_http  TEXT NOT NULL DEFAULT '',
    entrypoint_https TEXT NOT NULL DEFAULT '',
    cert_resolver    TEXT NOT NULL DEFAULT '',
    upstream_host    TEXT NOT NULL DEFAULT ''
);

INSERT INTO proxy_integration_settings (id) VALUES (1);

CREATE TABLE proxy_route_status (
    domain        TEXT PRIMARY KEY,
    target        TEXT NOT NULL DEFAULT '',
    file_name     TEXT NOT NULL DEFAULT '',
    written       INTEGER NOT NULL DEFAULT 0,
    written_at    TEXT NOT NULL DEFAULT '',
    proxy_loaded  TEXT NOT NULL DEFAULT 'unknown',
    reachable     INTEGER NOT NULL DEFAULT 0,
    status_code   INTEGER NOT NULL DEFAULT 0,
    cert_issuer   TEXT NOT NULL DEFAULT '',
    cert_trusted  INTEGER NOT NULL DEFAULT 0,
    cert_lets_encrypt INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT NOT NULL DEFAULT '',
    verified_at   TEXT NOT NULL DEFAULT ''
);
