CREATE TABLE team_invites (
    id          TEXT PRIMARY KEY,
    team_id     TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email       TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL CHECK (role IN ('admin', 'developer', 'viewer')),
    token_hash  TEXT NOT NULL UNIQUE,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    expires_at  TEXT NOT NULL,
    accepted_at TEXT,
    accepted_by TEXT,
    revoked_at  TEXT
);

CREATE INDEX idx_team_invites_team_id ON team_invites (team_id);
