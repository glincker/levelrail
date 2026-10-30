-- OAuth 2.1 authorization server for the remote MCP endpoint: pre-registered
-- clients, pending consent requests with their one-time codes, grants and the
-- rotating refresh tokens. Only hashes of codes and refresh tokens are stored.
CREATE TABLE mcp_oauth_clients (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    redirect_uris TEXT NOT NULL,
    scopes        TEXT NOT NULL,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL
);

CREATE TABLE mcp_oauth_requests (
    id             TEXT PRIMARY KEY,
    client_id      TEXT NOT NULL,
    redirect_uri   TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    scopes         TEXT NOT NULL,
    resource       TEXT NOT NULL,
    state          TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    expires_at     INTEGER NOT NULL,
    user_id        TEXT NOT NULL DEFAULT '',
    decided_at     INTEGER,
    approved       INTEGER NOT NULL DEFAULT 0,
    code_hash      TEXT NOT NULL DEFAULT '',
    code_expires_at INTEGER NOT NULL DEFAULT 0,
    code_used_at   INTEGER
);
CREATE INDEX idx_mcp_oauth_requests_code ON mcp_oauth_requests (code_hash) WHERE code_hash <> '';

CREATE TABLE mcp_oauth_grants (
    id         TEXT PRIMARY KEY,
    client_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL,
    scopes     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    request_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    revoked_at INTEGER
);
CREATE INDEX idx_mcp_oauth_grants_client ON mcp_oauth_grants (client_id);

CREATE TABLE mcp_oauth_tokens (
    token_id   TEXT PRIMARY KEY,
    grant_id   TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_mcp_oauth_tokens_grant ON mcp_oauth_tokens (grant_id);

CREATE TABLE mcp_oauth_refresh (
    hash       TEXT PRIMARY KEY,
    grant_id   TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at    INTEGER
);
CREATE INDEX idx_mcp_oauth_refresh_grant ON mcp_oauth_refresh (grant_id);
