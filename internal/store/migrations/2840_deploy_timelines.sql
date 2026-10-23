-- One frozen timeline per settled deploy, so a finished deploy can be replayed
-- after the sources it was assembled from (traffic samples, hook runs) age out.
-- No FK to deploy_attempts, same as deploy_trace_steps; the janitor deletes both.
CREATE TABLE deploy_timelines (
    attempt_id   TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    frozen_at    TEXT NOT NULL,
    payload      TEXT NOT NULL
);

CREATE INDEX idx_deploy_timelines_frozen ON deploy_timelines (frozen_at);
