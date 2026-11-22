-- Cloud accounts: one stored credential per provider account, shareable by grant.
-- The secret itself stays in service_secret_values; credential_ref only names it.
CREATE TABLE cloud_accounts (
    id               TEXT PRIMARY KEY,
    provider         TEXT NOT NULL,
    display_name     TEXT NOT NULL DEFAULT '',
    auth_method      TEXT NOT NULL DEFAULT 'api_key',
    config           TEXT NOT NULL DEFAULT '{}',
    credential_ref   TEXT NOT NULL DEFAULT '',
    team_id          TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'active',
    created_by       TEXT NOT NULL DEFAULT '',
    last_verified_at TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX cloud_accounts_provider ON cloud_accounts (provider);

CREATE TABLE cloud_grants (
    id             TEXT PRIMARY KEY,
    account_id     TEXT NOT NULL REFERENCES cloud_accounts (id) ON DELETE CASCADE,
    resource_kind  TEXT NOT NULL DEFAULT 'account',
    resource_ref   TEXT NOT NULL DEFAULT '',
    principal_type TEXT NOT NULL,
    principal_id   TEXT NOT NULL,
    actions        TEXT NOT NULL DEFAULT '[]',
    granted_by     TEXT NOT NULL DEFAULT '',
    expires_at     TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL,
    UNIQUE (account_id, resource_kind, resource_ref, principal_type, principal_id)
);

CREATE INDEX cloud_grants_principal ON cloud_grants (principal_type, principal_id);

ALTER TABLE node_provisions ADD COLUMN account_id TEXT NOT NULL DEFAULT '';
ALTER TABLE node_provisions ADD COLUMN created_by TEXT NOT NULL DEFAULT '';
ALTER TABLE node_provisions ADD COLUMN team_id TEXT NOT NULL DEFAULT '';

-- Each stored node-provider token becomes "<provider>-default", pointing at the
-- same secret key, so nothing is re-sealed. Idempotent by id.
INSERT OR IGNORE INTO cloud_accounts (id, provider, display_name, auth_method, credential_ref, created_at, updated_at)
SELECT substr(service_name, 15) || '-default', substr(service_name, 15), substr(service_name, 15), 'api_key', service_name,
       strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
FROM service_secret_values
WHERE service_name LIKE 'node-provider/%' AND env_key = 'token';

UPDATE node_provisions SET account_id = provider || '-default' WHERE account_id = '';
