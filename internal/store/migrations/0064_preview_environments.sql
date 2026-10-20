-- Preview environments per pull request (docs/roadmap.md). Opt-in per
-- git source, off by default: preview_enabled lives on
-- service_git_sources since a preview deploy only ever makes sense for
-- an app that already has a connected source to build a PR's branch
-- from.
ALTER TABLE service_git_sources ADD COLUMN preview_enabled INTEGER NOT NULL DEFAULT 0;

-- One row per (app, PR): the preview app a pull request currently owns,
-- so a synchronize push finds and reuses the same preview instead of
-- minting a second one, and a closed/merged PR has something concrete
-- to tear down. preview_app_id is the store.App/DesiredService name the
-- preview deployed under ("<app_name>-pr-<pr_number>"); environment_id
-- is nullable because tagging a preview with its shared Environment is
-- best-effort (see internal/api's ensurePreviewEnvironmentTier), not a
-- precondition for the preview itself being reachable.
CREATE TABLE preview_environments (
    id             TEXT PRIMARY KEY,
    app_name       TEXT NOT NULL,
    pr_number      INTEGER NOT NULL,
    preview_app_id TEXT NOT NULL,
    environment_id TEXT,
    branch         TEXT NOT NULL,
    head_sha       TEXT NOT NULL,
    domain         TEXT,
    status         TEXT NOT NULL,
    status_reason  TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE UNIQUE INDEX ux_preview_environments_app_pr ON preview_environments (app_name, pr_number);
CREATE INDEX idx_preview_environments_app_name ON preview_environments (app_name);
