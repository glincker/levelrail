-- Commit metadata for the cross-app deployments list, plus a keyset index
-- so the newest-first cursor scan does not sort the whole table.
ALTER TABLE deploy_attempts ADD COLUMN branch TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN commit_message TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN author TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_deploy_attempts_started_id ON deploy_attempts (started_at DESC, id DESC);
