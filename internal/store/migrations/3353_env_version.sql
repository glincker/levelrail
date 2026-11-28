-- Bumped on every env, secret declaration or vault reference change so a stale writer is detected.
ALTER TABLE desired_services ADD COLUMN env_version INTEGER NOT NULL DEFAULT 0;
