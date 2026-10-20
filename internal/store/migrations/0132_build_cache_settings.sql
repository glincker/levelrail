-- Remote BuildKit cache per app on a storage destination (a backup_targets
-- row). app_name '' is the global default. last_* fields are cache hygiene
-- signals written by the build path, never credentials.
CREATE TABLE build_cache_settings (
    app_name         TEXT PRIMARY KEY,
    target_id        TEXT NOT NULL REFERENCES backup_targets(id) ON DELETE CASCADE,
    enabled          INTEGER NOT NULL DEFAULT 1,
    mode             TEXT NOT NULL DEFAULT 'max' CHECK (mode IN ('min', 'max')),
    last_build_at    TEXT NOT NULL DEFAULT '',
    last_result      TEXT NOT NULL DEFAULT '',
    last_warning     TEXT NOT NULL DEFAULT '',
    last_cleared_at  TEXT NOT NULL DEFAULT '',
    updated_at       TEXT NOT NULL
);
