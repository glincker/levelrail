-- Stable requester identity for deploy approvals: the session user or the
-- token owner, resolved at request time. Empty text means an ownerless token;
-- NULL marks older rows, which fall back to a decide-time lookup.
ALTER TABLE deploy_approvals ADD COLUMN requester_user_id TEXT;
