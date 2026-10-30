-- A pending approval names the release it will apply (ADR 050); null for tag based approvals.
ALTER TABLE deploy_approvals ADD COLUMN release_id TEXT;
