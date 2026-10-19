CREATE TABLE user_preferences (
    user_id      TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    timezone     TEXT NOT NULL DEFAULT '',
    language     TEXT NOT NULL DEFAULT '',
    theme        TEXT NOT NULL DEFAULT '',
    landing_page TEXT NOT NULL DEFAULT '',
    gravatar     INTEGER NOT NULL DEFAULT 0,
    updated_at   TEXT NOT NULL
);
