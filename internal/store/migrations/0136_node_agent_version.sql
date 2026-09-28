-- What the node's agent reported about itself when its session opened.
-- Empty until an agent new enough to send it connects.
ALTER TABLE nodes ADD COLUMN agent_version TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_commit TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_os TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_arch TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN agent_reported_at TEXT;
