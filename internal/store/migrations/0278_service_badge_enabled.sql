-- Per-app opt-in for the public deploy status badge (GET
-- /api/v1/apps/{name}/badge.svg): off by default so a private app's
-- deploy status is never exposed without an explicit choice, unlike
-- exec_enabled's default-true shape.
ALTER TABLE desired_services ADD COLUMN badge_enabled INTEGER NOT NULL DEFAULT 0;
