-- The App's slug (its github.com/apps/<slug> URL segment) was only ever
-- available transiently from the one-time manifest exchange response,
-- never persisted, so there was no way to build a fresh
-- "install on another org" link after initial setup. Nullable: existing
-- rows predate this and get it lazily (handleListGitHubAppInstallations).
ALTER TABLE github_app_connections ADD COLUMN slug TEXT;

-- A GitHub App installation is scoped to one account or org at a time,
-- but github_app_connections.installation_id/account_login only ever
-- held one, so installing on a second org silently clobbered the first.
-- This table makes installations one-to-many; the single columns on
-- github_app_connections stay (nothing drops them) but are no longer
-- written to after this change, only read as a migration source below.
CREATE TABLE github_app_installations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    installation_id INTEGER NOT NULL UNIQUE,
    account_login   TEXT NOT NULL,
    account_type    TEXT NOT NULL CHECK (account_type IN ('user', 'organization')),
    connected_at    TEXT NOT NULL
);

INSERT INTO github_app_installations (installation_id, account_login, account_type, connected_at)
SELECT installation_id, account_login, 'organization', created_at
FROM github_app_connections
WHERE id = 1 AND installation_id IS NOT NULL AND account_login IS NOT NULL;
