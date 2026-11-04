-- Per-user preferences as small JSON documents keyed by name (appearance first), so a user's look follows them across browsers.
CREATE TABLE user_preference_docs (
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, key)
);
