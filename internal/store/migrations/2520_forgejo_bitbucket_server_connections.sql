-- Forgejo and Bitbucket Server connections: drop the provider CHECK list so a new provider is a code-only change (validated in the api package).
CREATE TABLE git_provider_connections_new (
    id           TEXT PRIMARY KEY,
    provider     TEXT NOT NULL,
    label        TEXT NOT NULL,
    host         TEXT NOT NULL,
    instance_url TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    secrets_key  TEXT NOT NULL,
    account_id   TEXT NOT NULL DEFAULT '',
    account_name TEXT NOT NULL DEFAULT '',
    scopes       TEXT NOT NULL DEFAULT '',
    is_default   INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL
);

INSERT INTO git_provider_connections_new (id, provider, label, host, instance_url, client_id, secrets_key, account_id, account_name, scopes, is_default, created_by, created_at)
SELECT id, provider, label, host, instance_url, client_id, secrets_key, account_id, account_name, scopes, is_default, created_by, created_at FROM git_provider_connections;

DROP TABLE git_provider_connections; -- compat-break: table rebuilt under the same name to widen its provider check

ALTER TABLE git_provider_connections_new RENAME TO git_provider_connections;

CREATE INDEX git_provider_connections_provider_host ON git_provider_connections (provider, host);
