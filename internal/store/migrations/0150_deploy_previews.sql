-- Deploy preview screenshot metadata. Thumbnails live on disk under
-- DataDir/previews, never in this database.
CREATE TABLE IF NOT EXISTS deploy_previews (
    deployment_id   TEXT PRIMARY KEY,
    app_name        TEXT NOT NULL,
    path            TEXT NOT NULL DEFAULT '/',
    bytes           INTEGER NOT NULL DEFAULT 0,
    width           INTEGER NOT NULL DEFAULT 0,
    height          INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    detail          TEXT NOT NULL DEFAULT '',
    http_status     INTEGER NOT NULL DEFAULT 0,
    captured_at     TEXT NOT NULL,
    last_viewed_at  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_deploy_previews_app_time ON deploy_previews (app_name, captured_at DESC);

CREATE TABLE IF NOT EXISTS deploy_preview_settings (
    app_name    TEXT PRIMARY KEY,
    enabled     INTEGER NOT NULL DEFAULT 0,
    path        TEXT NOT NULL DEFAULT '/',
    wait_ms     INTEGER NOT NULL DEFAULT 0,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS deploy_preview_state (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL DEFAULT ''
);
