-- GitHub Enterprise Server self-hosted support: which GitHub instance
-- this control plane's App is registered against. Defaults to
-- github.com so every existing connection stays correct with no
-- backfill, the same single-field pattern gitlab_app_connections'
-- instance_url (migrations/0044) already established rather than
-- Dokploy's own external/internal URL split (see
-- docs/design/git-provider-integrations.md for why).
ALTER TABLE github_app_connections
    ADD COLUMN instance_url TEXT NOT NULL DEFAULT 'https://github.com';
