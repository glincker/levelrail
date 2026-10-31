-- Last rolling-deploy progress per app, so a control plane restart does not
-- blank the dashboard or CLI. payload is the JSON the rollout API serves.
CREATE TABLE IF NOT EXISTS rollout_progress (
    service_name TEXT PRIMARY KEY,
    payload      TEXT NOT NULL,
    updated_at   TEXT NOT NULL DEFAULT ''
);
