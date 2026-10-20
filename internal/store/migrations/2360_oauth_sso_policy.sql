-- Group to role mapping, role sync, JIT and domain policy for OAuth and OIDC
-- logins, the same vocabulary SAML connections use. Defaults are least
-- privilege: no JIT, no role sync, viewer for unmapped users. Providers that
-- are already enabled keep today's behavior (a new email is provisioned).
ALTER TABLE oauth_provider_settings ADD COLUMN groups_claim TEXT NOT NULL DEFAULT 'groups';
ALTER TABLE oauth_provider_settings ADD COLUMN sync_roles INTEGER NOT NULL DEFAULT 0;
ALTER TABLE oauth_provider_settings ADD COLUMN default_role_id TEXT NOT NULL DEFAULT 'role_viewer';
ALTER TABLE oauth_provider_settings ADD COLUMN jit_provisioning INTEGER NOT NULL DEFAULT 0;
ALTER TABLE oauth_provider_settings ADD COLUMN allowed_domains TEXT NOT NULL DEFAULT '[]';

UPDATE oauth_provider_settings SET jit_provisioning = 1 WHERE enabled = 1;
