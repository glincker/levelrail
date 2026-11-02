-- Optional Entra tenant id for the Microsoft provider. Empty keeps the shared
-- tenant, where a new account cannot be created from a sign-in.
ALTER TABLE oauth_provider_settings ADD COLUMN tenant_id TEXT NOT NULL DEFAULT '';
