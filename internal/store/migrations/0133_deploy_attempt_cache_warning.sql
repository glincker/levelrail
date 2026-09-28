-- Set when a build carried on without its remote cache (import or export
-- failed, or the build ran on a node that gets no bucket credentials).
ALTER TABLE deploy_attempts ADD COLUMN cache_warning TEXT NOT NULL DEFAULT '';
