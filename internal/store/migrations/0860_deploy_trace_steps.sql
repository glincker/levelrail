-- One row per pipeline step of a deploy attempt, build or not. step_key is
-- unique within an attempt so a retried step overwrites instead of piling up.
-- No FK to deploy_attempts: image deploys trace resolution before the row exists.
CREATE TABLE deploy_trace_steps (
    id          INTEGER PRIMARY KEY,
    attempt_id  TEXT NOT NULL,
    step_key    TEXT NOT NULL,
    step        TEXT NOT NULL,
    service     TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    attempts    INTEGER NOT NULL DEFAULT 1,
    started_at  TEXT NOT NULL,
    ended_at    TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    UNIQUE (attempt_id, step_key)
);

CREATE INDEX idx_deploy_trace_steps_attempt ON deploy_trace_steps (attempt_id, id);
