-- External HashiCorp Vault integration: platform-wide connection config,
-- mirroring registry_settings' (migrations/0085) single seeded row.
-- No credential column: the Vault token (auth_method = 'token') or
-- AppRole secret ID (auth_method = 'approle') goes through
-- internal/secrets instead, keyed by store.VaultSecretsKey(). role_id is
-- not itself sensitive (HashiCorp's own AppRole docs treat it like a
-- username), so it lives here in plain text alongside address/mount_path.
CREATE TABLE vault_settings (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    enabled     INTEGER NOT NULL DEFAULT 0,
    address     TEXT NOT NULL DEFAULT '',
    auth_method TEXT NOT NULL DEFAULT 'token',
    namespace   TEXT NOT NULL DEFAULT '',
    role_id     TEXT NOT NULL DEFAULT '',
    mount_path  TEXT NOT NULL DEFAULT 'secret',
    created_at  TEXT
);

INSERT INTO vault_settings (id) VALUES (1);
