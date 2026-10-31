-- A replay approval names the held deploy attempt it releases once approved; empty for other actions.
ALTER TABLE deploy_approvals ADD COLUMN held_attempt_id TEXT NOT NULL DEFAULT '';
