-- Managed nginx routes: a third integration mode and the proxy container, certificate directory and ACME webroot nginx needs.
CREATE TABLE proxy_integration_settings_new (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    mode             TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'traefik_file', 'nginx_file')),
    dynamic_dir      TEXT NOT NULL DEFAULT '',
    entrypoint_http  TEXT NOT NULL DEFAULT '',
    entrypoint_https TEXT NOT NULL DEFAULT '',
    cert_resolver    TEXT NOT NULL DEFAULT '',
    upstream_host    TEXT NOT NULL DEFAULT '',
    container        TEXT NOT NULL DEFAULT '',
    cert_dir         TEXT NOT NULL DEFAULT '',
    webroot          TEXT NOT NULL DEFAULT ''
);

INSERT INTO proxy_integration_settings_new (id, mode, dynamic_dir, entrypoint_http, entrypoint_https, cert_resolver, upstream_host)
SELECT id, mode, dynamic_dir, entrypoint_http, entrypoint_https, cert_resolver, upstream_host FROM proxy_integration_settings;

DROP TABLE proxy_integration_settings; -- compat-break: table rebuilt under the same name to widen its columns
ALTER TABLE proxy_integration_settings_new RENAME TO proxy_integration_settings;
