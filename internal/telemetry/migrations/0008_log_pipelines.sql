-- Per-app log pipeline: an ordered JSON list of rules applied to each line
-- before it is stored, broadcast or exported.
CREATE TABLE log_pipelines (
    resource_id TEXT PRIMARY KEY,
    rules_json  TEXT NOT NULL,
    updated_at  INTEGER NOT NULL
);
