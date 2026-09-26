-- Queued and canceled deploys: when a queued attempt entered the queue, who
-- canceled one, which newer attempt superseded it, and the per-app opt-in to
-- superseding older queued deploys of the same branch.
ALTER TABLE deploy_attempts ADD COLUMN queued_at TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN superseded_by TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN canceled_by TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN cancel_superseded INTEGER NOT NULL DEFAULT 0;
