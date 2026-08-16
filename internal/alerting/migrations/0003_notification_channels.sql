-- A notification channel is a global, connect-once notify destination,
-- attached by ID from deploy_notify_targets rows (migrations/0004)
-- instead of retyping a URL per app.
CREATE TABLE notification_channels (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL DEFAULT 'generic', -- 'generic' | 'slack' | 'discord' | 'telegram' | 'email'
    notify_url  TEXT NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,

    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
