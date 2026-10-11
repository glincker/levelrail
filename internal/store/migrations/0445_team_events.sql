CREATE TABLE team_events (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL,
    actor_id   TEXT NOT NULL DEFAULT '',
    actor_name TEXT NOT NULL DEFAULT '',
    action     TEXT NOT NULL,
    subject    TEXT NOT NULL DEFAULT '',
    detail     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX idx_team_events_team_created ON team_events (team_id, created_at DESC);
