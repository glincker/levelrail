-- Per-app settings for what the GitHub Actions deploy action reports back to the PR.
CREATE TABLE IF NOT EXISTS app_github_actions (
    service_name        TEXT PRIMARY KEY,
    environment         TEXT NOT NULL DEFAULT 'production',
    report_deployments  INTEGER NOT NULL DEFAULT 1,
    report_checks       INTEGER NOT NULL DEFAULT 1,
    updated_at          TEXT NOT NULL
);
