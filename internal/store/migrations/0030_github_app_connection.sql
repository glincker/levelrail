-- Single-row GitHub App connection (this platform's own GitHub App
-- registration, not a per-app git config). Unlike ingress_settings
-- (migrations/0024, always seeded so id=1 always exists), this table
-- starts empty: "no row" is the real, valid "never registered" state,
-- the same "absence is real, permanent state" reasoning
-- ingress_settings.primary_domain's own NULL already carries at the
-- column level, applied here at the whole-row level instead because
-- there is nothing meaningful to default app_id/client_id to before an
-- operator actually completes the manifest flow.
--
-- No credential columns here, matching backup_targets' own
-- (migrations/0018) split: app_id/client_id/installation_id/
-- account_login are not secret (client_id is routinely embedded in
-- public authorize URLs, app_id is visible in the App's own public
-- github.com/apps/<slug> page), so they live here as plain columns.
-- client_secret, webhook_secret, and the App's PEM private key are the
-- three real secrets and go through internal/secrets.Manager instead,
-- keyed by store.GitHubAppSecretsKey() (internal/store/github_app.go),
-- the exact same envelope-encryption path backup target credentials
-- already use.
--
-- installation_id/account_login are nullable: the App can exist
-- (registered via the manifest flow) before it has been installed on
-- any account. A row with a non-NULL app_id/client_id and a NULL
-- installation_id means "App created, not yet installed"; both non-NULL
-- means "created and installed".
CREATE TABLE github_app_connections (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    app_id          INTEGER NOT NULL,
    client_id       TEXT NOT NULL,
    installation_id INTEGER,
    account_login   TEXT,
    created_at      TEXT NOT NULL
);
