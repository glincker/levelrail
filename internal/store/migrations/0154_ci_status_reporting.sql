-- Path filters and status reporting for deploy-on-push, commit-status
-- outcome on pipeline runs, and forge deployment records.
ALTER TABLE service_git_sources ADD COLUMN deploy_paths TEXT NOT NULL DEFAULT '[]';
ALTER TABLE service_git_sources ADD COLUMN deploy_paths_ignore TEXT NOT NULL DEFAULT '[]';
ALTER TABLE service_git_sources ADD COLUMN report_status INTEGER NOT NULL DEFAULT 1;

ALTER TABLE pipeline_runs ADD COLUMN report_provider TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN report_state TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN report_url TEXT NOT NULL DEFAULT '';
ALTER TABLE pipeline_runs ADD COLUMN report_warning TEXT NOT NULL DEFAULT '';

CREATE TABLE forge_deployments (
    id TEXT PRIMARY KEY,
    app_name TEXT NOT NULL,
    environment TEXT NOT NULL,
    provider TEXT NOT NULL,
    external_id INTEGER NOT NULL,
    commit_sha TEXT NOT NULL,
    state TEXT NOT NULL,
    warning TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_forge_deployments_app_env ON forge_deployments (app_name, environment, created_at);
