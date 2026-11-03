-- Per-user first-run flow position. flow_version lets a redesigned flow tell old rows apart.
CREATE TABLE user_onboarding (
    user_id      TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    state        TEXT NOT NULL DEFAULT 'not_started',
    current_step TEXT NOT NULL DEFAULT 'welcome',
    flow_version INTEGER NOT NULL DEFAULT 1,
    started_at   TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    dismissed_at TEXT NOT NULL DEFAULT '',
    updated_at   TEXT NOT NULL
);
