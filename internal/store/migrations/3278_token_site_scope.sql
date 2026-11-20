-- Site scope for API tokens: a token bound to one Pages site may only upload
-- and read deploys of that site, so a CI pipeline never holds wider access.
CREATE TABLE IF NOT EXISTS api_token_site_scopes (
    token_id  TEXT PRIMARY KEY,
    site_name TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_api_token_site_scopes_site ON api_token_site_scopes (site_name);
