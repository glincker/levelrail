-- Widens the provider CHECK constraints (0700) to admit dropbox, kakao and patreon.
CREATE TABLE oauth_provider_settings_new (
    provider             TEXT PRIMARY KEY CHECK (provider IN ('google', 'github', 'oidc', 'microsoft', 'gitlab', 'bitbucket', 'discord', 'slack', 'linkedin', 'twitch', 'facebook', 'linear', 'vercel', 'huggingface', 'figma', 'salesforce', 'codeberg', 'cognito', 'dropbox', 'kakao', 'patreon')),
    enabled              INTEGER NOT NULL DEFAULT 0,
    client_id            TEXT,
    allowed_email_domain TEXT,
    issuer_url           TEXT,
    display_name         TEXT,
    groups_claim         TEXT NOT NULL DEFAULT 'groups',
    sync_roles           INTEGER NOT NULL DEFAULT 0,
    default_role_id      TEXT NOT NULL DEFAULT 'role_viewer',
    jit_provisioning     INTEGER NOT NULL DEFAULT 0,
    allowed_domains      TEXT NOT NULL DEFAULT '[]'
);

INSERT INTO oauth_provider_settings_new (provider, enabled, client_id, allowed_email_domain, issuer_url, display_name, groups_claim, sync_roles, default_role_id, jit_provisioning, allowed_domains)
SELECT provider, enabled, client_id, allowed_email_domain, issuer_url, display_name, groups_claim, sync_roles, default_role_id, jit_provisioning, allowed_domains FROM oauth_provider_settings;

DROP TABLE oauth_provider_settings;

ALTER TABLE oauth_provider_settings_new RENAME TO oauth_provider_settings;

INSERT INTO oauth_provider_settings (provider, enabled) VALUES
    ('dropbox', 0), ('kakao', 0), ('patreon', 0);

CREATE TABLE user_oauth_identities_new (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('google', 'github', 'oidc', 'microsoft', 'gitlab', 'bitbucket', 'discord', 'slack', 'linkedin', 'twitch', 'facebook', 'linear', 'vercel', 'huggingface', 'figma', 'salesforce', 'codeberg', 'cognito', 'dropbox', 'kakao', 'patreon')),
    provider_user_id TEXT NOT NULL,
    created_at       TEXT NOT NULL
);

INSERT INTO user_oauth_identities_new (id, user_id, provider, provider_user_id, created_at)
SELECT id, user_id, provider, provider_user_id, created_at FROM user_oauth_identities;

DROP TABLE user_oauth_identities;

ALTER TABLE user_oauth_identities_new RENAME TO user_oauth_identities;

CREATE UNIQUE INDEX ux_oauth_identities_provider_account ON user_oauth_identities (provider, provider_user_id);
CREATE UNIQUE INDEX ux_oauth_identities_user_provider ON user_oauth_identities (user_id, provider);
