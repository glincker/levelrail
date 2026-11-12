-- Named cloud account connections (preview). Config holds non-secret settings as JSON;
-- credential_ref is a reference only, the credential itself is never stored here.
CREATE TABLE cloud_connections (
    name              TEXT PRIMARY KEY,
    provider          TEXT NOT NULL,
    team_id           TEXT NOT NULL DEFAULT '',
    auth_method       TEXT NOT NULL,
    config            TEXT NOT NULL DEFAULT '{}',
    credential_ref    TEXT NOT NULL DEFAULT '',
    created_by        TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    last_checked_at   TEXT NOT NULL DEFAULT '',
    last_check_class  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX cloud_connections_team_idx ON cloud_connections (team_id);
