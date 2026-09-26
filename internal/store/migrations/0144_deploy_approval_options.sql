-- Carries a protected deploy's request options through to approval.
ALTER TABLE deploy_approvals ADD COLUMN freeze_override TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_approvals ADD COLUMN pull INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deploy_approvals ADD COLUMN include_env INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deploy_approvals ADD COLUMN promote_env TEXT NOT NULL DEFAULT '';
