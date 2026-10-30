-- One-shot scheduled deploys (image or git ref) with their own id, so an
-- app can have several and each can be listed, read and canceled.
CREATE TABLE IF NOT EXISTS scheduled_deploys (
    id               TEXT PRIMARY KEY,
    service_name     TEXT NOT NULL,
    kind             TEXT NOT NULL,
    image            TEXT NOT NULL DEFAULT '',
    ref              TEXT NOT NULL DEFAULT '',
    run_at           TEXT NOT NULL,
    status           TEXT NOT NULL,
    reason           TEXT NOT NULL DEFAULT '',
    approval_id      TEXT NOT NULL DEFAULT '',
    created_by_type  TEXT NOT NULL DEFAULT '',
    created_by       TEXT NOT NULL DEFAULT '',
    created_by_name  TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    finished_at      TEXT
);
CREATE INDEX IF NOT EXISTS idx_scheduled_deploys_due ON scheduled_deploys (status, run_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_deploys_service ON scheduled_deploys (service_name, run_at DESC);
