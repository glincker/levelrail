-- Stable requester identity for stack approvals: the user behind the session
-- or the token owner. Empty on older rows, which then cannot be approved.
ALTER TABLE stack_revisions ADD COLUMN requester_user_id TEXT NOT NULL DEFAULT '';
