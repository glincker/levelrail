-- Widens provider CHECK constraints (0690_more_oauth_providers.sql) to admit
-- linear, vercel, huggingface, figma, salesforce, codeberg and cognito.
-- issuer_url doubles as the Cognito user pool id (region is its prefix).
CREATE TABLE oauth_provider_settings_new (
    provider             TEXT PRIMARY KEY CHECK (provider IN ('google', 'github', 'oidc', 'microsoft', 'gitlab', 'bitbucket', 'discord', 'slack', 'linkedin', 'twitch', 'facebook', 'linear', 'vercel', 'huggingface', 'figma', 'salesforce', 'codeberg', 'cognito')),
    enabled              INTEGER NOT NULL DEFAULT 0,
    client_id            TEXT,
    allowed_email_domain TEXT,
    issuer_url           TEXT,
    display_name         TEXT
);

INSERT INTO oauth_provider_settings_new (provider, enabled, client_id, allowed_email_domain, issuer_url, display_name)
SELECT provider, enabled, client_id, allowed_email_domain, issuer_url, display_name FROM oauth_provider_settings;

DROP TABLE oauth_provider_settings;

ALTER TABLE oauth_provider_settings_new RENAME TO oauth_provider_settings;

INSERT INTO oauth_provider_settings (provider, enabled) VALUES
    ('linear', 0), ('vercel', 0), ('huggingface', 0), ('figma', 0),
    ('salesforce', 0), ('codeberg', 0), ('cognito', 0);

CREATE TABLE user_oauth_identities_new (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         TEXT NOT NULL CHECK (provider IN ('google', 'github', 'oidc', 'microsoft', 'gitlab', 'bitbucket', 'discord', 'slack', 'linkedin', 'twitch', 'facebook', 'linear', 'vercel', 'huggingface', 'figma', 'salesforce', 'codeberg', 'cognito')),
    provider_user_id TEXT NOT NULL,
    created_at       TEXT NOT NULL
);

INSERT INTO user_oauth_identities_new (id, user_id, provider, provider_user_id, created_at)
SELECT id, user_id, provider, provider_user_id, created_at FROM user_oauth_identities;

DROP TABLE user_oauth_identities;

ALTER TABLE user_oauth_identities_new RENAME TO user_oauth_identities;

CREATE UNIQUE INDEX ux_oauth_identities_provider_account ON user_oauth_identities (provider, provider_user_id);
CREATE UNIQUE INDEX ux_oauth_identities_user_provider ON user_oauth_identities (user_id, provider);
