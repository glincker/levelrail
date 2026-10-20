-- Sign in with a code, and new-device approval. Codes are stored only as a
-- salted HMAC; browser_hash is the SHA-256 of the requesting browser's
-- binding cookie. user_id is empty for a challenge on an unknown account.
CREATE TABLE IF NOT EXISTS login_code_challenges (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL DEFAULT '',
    browser_hash TEXT NOT NULL,
    code_hash    TEXT NOT NULL,
    salt         TEXT NOT NULL,
    requester_ip TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    attempts     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    resolved_at  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_login_code_challenges_browser ON login_code_challenges (browser_hash, status);
CREATE INDEX IF NOT EXISTS idx_login_code_challenges_user ON login_code_challenges (user_id, status);
CREATE INDEX IF NOT EXISTS idx_login_code_challenges_expiry ON login_code_challenges (status, expires_at);

CREATE TABLE IF NOT EXISTS login_approvals (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    browser_hash TEXT NOT NULL,
    requester_ip TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    decided_by   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    resolved_at  TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_login_approvals_browser ON login_approvals (browser_hash);
CREATE INDEX IF NOT EXISTS idx_login_approvals_user ON login_approvals (user_id, status);
CREATE INDEX IF NOT EXISTS idx_login_approvals_expiry ON login_approvals (status, expires_at);
