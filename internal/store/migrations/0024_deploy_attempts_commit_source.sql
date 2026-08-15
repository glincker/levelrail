ALTER TABLE deploy_attempts ADD COLUMN commit_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN source TEXT NOT NULL DEFAULT '';
