-- Per-app git source (TASKS.md 1.7's own deferred follow-up: internal/webhook's
-- own package doc comment scopes it to "a single app... deploying from a
-- git push," not a multi-tenant registry). This table is that registry:
-- one row per app that has a repo connected, so a git push can trigger
-- that specific app's build without server-side env vars or a restart.
--
-- No foreign key to desired_services (name): mirrors service_secrets'
-- own bare-TEXT-primary-key choice (migrations/0006_service_secrets.sql),
-- not backup_targets' REFERENCES-and-block-on-delete choice
-- (migrations/0018_backup_targets.sql). A backup target's credentials
-- guard real, external, billable storage that a stray delete could
-- orphan; a git source is app-scoped, disposable config the same way an
-- app's own secret_env values already are, and DeleteDesiredService
-- (internal/store/service.go) already leaves service_secret_values rows
-- behind uncleaned on app delete, a known, accepted gap this table
-- inherits rather than a new one it introduces.
--
-- build_type/build_path mirror triggerBuildBuildInput
-- (internal/api/builds.go): a git-push-triggered deploy needs to know
-- how to build the checkout the same way a manual build trigger's
-- request body already supplies per call, but a stored git source has
-- no per-call request body to read that from, so it's persisted here
-- instead. build_type is never empty (internal/api validates it against
-- spec.BuildDockerfile/BuildRailpack/BuildStatic before ever writing a
-- row, spec.BuildCompose rejected the same way handleTriggerBuild
-- already rejects it for a manual trigger).
CREATE TABLE service_git_sources (
    service_name TEXT PRIMARY KEY,
    repo_url     TEXT NOT NULL,
    branch       TEXT NOT NULL,
    build_type   TEXT NOT NULL,
    build_path   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
