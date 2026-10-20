CREATE TABLE api_token_scopes (
    token_id   TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    project_id TEXT NOT NULL DEFAULT '',
    role       TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'developer', 'viewer'))
);
