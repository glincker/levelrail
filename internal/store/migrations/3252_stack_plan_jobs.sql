-- Async cloud plan jobs of a stack. Rows hold the masked plan result and codes only,
-- never state or credentials, and are pruned after cloud.plan_job_retention_hours.
CREATE TABLE stack_plan_jobs (
    id            TEXT PRIMARY KEY,
    stack         TEXT NOT NULL,
    status        TEXT NOT NULL,
    code          TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    result        BLOB,
    cloud_hashes  TEXT NOT NULL DEFAULT '',
    actor         TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    started_at    TEXT NOT NULL DEFAULT '',
    finished_at   TEXT NOT NULL DEFAULT '',
    heartbeat_at  TEXT NOT NULL
);
CREATE INDEX idx_stack_plan_jobs_stack ON stack_plan_jobs (stack, created_at);
