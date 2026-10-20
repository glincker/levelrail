-- Instance edge settings kept in the database so the dashboard can change them; an APP_INGRESS_* env var still wins.
-- NULL limit columns mean "use the built-in default".
CREATE TABLE ingress_edge_settings (
    id                        INTEGER PRIMARY KEY CHECK (id = 1),
    trusted_proxies           TEXT NOT NULL DEFAULT '',
    max_header_bytes          INTEGER CHECK (max_header_bytes IS NULL OR max_header_bytes >= 0),
    max_body_bytes            INTEGER CHECK (max_body_bytes IS NULL OR max_body_bytes >= 0),
    read_header_timeout_secs  INTEGER CHECK (read_header_timeout_secs IS NULL OR read_header_timeout_secs >= 0),
    idle_timeout_secs         INTEGER CHECK (idle_timeout_secs IS NULL OR idle_timeout_secs >= 0),
    updated_at                TEXT NOT NULL DEFAULT ''
);
INSERT INTO ingress_edge_settings (id) VALUES (1);

CREATE TABLE app_ingress_limits (
    app_name       TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    max_body_bytes INTEGER NOT NULL CHECK (max_body_bytes > 0),
    updated_at     TEXT NOT NULL
);
