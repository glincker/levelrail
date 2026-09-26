-- Digest-truthful deploys, the stale-deploy guard and deploy freeze windows
-- (ADR 002 addendum, docs/deploy-safety.md).

-- image_id is the local content ID a build produced; image_id_ref is the
-- exact image string it was resolved for, so a later image change can never
-- pair a new tag with a stale ID.
ALTER TABLE desired_services ADD COLUMN image_id TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN image_id_ref TEXT NOT NULL DEFAULT '';

ALTER TABLE deploy_attempts ADD COLUMN image_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN digest_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN rollout_state TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN running_image_id TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE deploy_attempts ADD COLUMN reason TEXT NOT NULL DEFAULT '';
ALTER TABLE deploy_attempts ADD COLUMN held_request TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_deploy_attempts_status ON deploy_attempts(status);

-- One row per service: next_sequence hands out trigger order, the applied_*
-- columns describe the last deploy that actually wrote desired state.
CREATE TABLE IF NOT EXISTS deploy_cursors (
    service_name       TEXT PRIMARY KEY,
    next_sequence      INTEGER NOT NULL DEFAULT 0,
    applied_sequence   INTEGER NOT NULL DEFAULT 0,
    applied_commit     TEXT NOT NULL DEFAULT '',
    applied_before     TEXT NOT NULL DEFAULT '',
    applied_commit_at  TEXT NOT NULL DEFAULT '',
    updated_at         TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- scope is 'global' or 'app:<service name>'.
CREATE TABLE IF NOT EXISTS deploy_freeze_windows (
    id               TEXT PRIMARY KEY,
    scope            TEXT NOT NULL,
    cron             TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    reason           TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX IF NOT EXISTS idx_deploy_freeze_windows_scope ON deploy_freeze_windows(scope);
