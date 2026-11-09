-- Per-app build settings a git or manual build applies: Dockerfile target
-- stage, layer cache off, and the expected Railpack provider. Builder and
-- Dockerfile path stay on git_sources so there is one source of truth.
CREATE TABLE app_build_settings (
    app_name   TEXT PRIMARY KEY,
    target     TEXT NOT NULL DEFAULT '',
    no_cache   INTEGER NOT NULL DEFAULT 0,
    provider   TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS trg_desired_services_drop_build_settings
AFTER DELETE ON desired_services
BEGIN
    DELETE FROM app_build_settings WHERE app_name = OLD.name;
END;
