-- Platform-wide built-in container registry config: whether the operator
-- wants Levelrail's own registry:2 container running, mirroring
-- cloudflare_tunnel_settings' (migrations/0049) single seeded row. No
-- password column: the generated registry password goes through
-- internal/secrets instead, keyed by store.RegistrySettingsSecretsKey().
-- No status column either, same reasoning cloudflare_tunnel_settings'
-- own comment gives: derived state lives in reconcile_status.
CREATE TABLE registry_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    enabled    INTEGER NOT NULL DEFAULT 0,
    host       TEXT,
    username   TEXT,
    created_at TEXT
);

INSERT INTO registry_settings (id) VALUES (1);
