-- trigger_mode: which pushes to a connected git source actually deploy.
-- "push" (default, matches every git source created before this column
-- existed) deploys on every push to branch; "release" deploys only on a
-- tag ref push, or a GitHub "release" event with action "published"
-- (internal/api/git_webhook.go). Plain string, not JSON, same shape
-- build_type already uses on this table (0029).
ALTER TABLE service_git_sources ADD COLUMN trigger_mode TEXT NOT NULL DEFAULT 'push';
