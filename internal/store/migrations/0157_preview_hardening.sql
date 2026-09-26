-- Per-app preview policy, and the columns the PR comment upsert and the
-- fork gate need on each preview row.
CREATE TABLE preview_app_settings (
    app_name            TEXT PRIMARY KEY,
    on_limit            TEXT NOT NULL DEFAULT 'evict_oldest',
    allow_fork_previews INTEGER NOT NULL DEFAULT 0,
    ttl_hours           INTEGER NOT NULL DEFAULT 0,
    updated_at          TEXT NOT NULL
);

ALTER TABLE preview_environments ADD COLUMN comment_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_environments ADD COLUMN is_fork INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_environments ADD COLUMN head_repo TEXT NOT NULL DEFAULT '';
