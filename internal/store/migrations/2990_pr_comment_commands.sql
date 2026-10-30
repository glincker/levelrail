-- Opt-in pull request comment commands (/levelrail redeploy, rollback, status).
-- Both flags default to off; the log makes a redelivered comment a no-op.
ALTER TABLE service_git_sources ADD COLUMN pr_comment_commands INTEGER NOT NULL DEFAULT 0;
ALTER TABLE service_git_sources ADD COLUMN pr_comment_commands_forks INTEGER NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS pr_comment_commands (
    service_name TEXT NOT NULL,
    comment_id   INTEGER NOT NULL,
    actor        TEXT NOT NULL,
    command      TEXT NOT NULL,
    pr_number    INTEGER NOT NULL,
    created_at   TEXT NOT NULL,
    PRIMARY KEY (service_name, comment_id)
);
CREATE INDEX IF NOT EXISTS idx_pr_comment_commands_created ON pr_comment_commands (created_at);
