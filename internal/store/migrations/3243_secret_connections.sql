-- Named external secret connections. Non-secret settings live in config (JSON);
-- the credential is held by the encrypted secret store, never in this table.
CREATE TABLE secret_connections (
    name           TEXT PRIMARY KEY,
    type           TEXT NOT NULL,
    config         TEXT NOT NULL DEFAULT '{}',
    credential_ref TEXT NOT NULL DEFAULT '',
    created_by     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
