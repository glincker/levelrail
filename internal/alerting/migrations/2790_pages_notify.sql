-- Per-site Pages deploy notification switch and event filter. A site with no
-- row keeps the default of notifying on every deploy outcome.
CREATE TABLE IF NOT EXISTS pages_notify_settings (
    resource_id TEXT PRIMARY KEY,
    enabled     INTEGER NOT NULL DEFAULT 1,
    on_success  INTEGER NOT NULL DEFAULT 1,
    on_failure  INTEGER NOT NULL DEFAULT 1,
    updated_at  TEXT NOT NULL
);
