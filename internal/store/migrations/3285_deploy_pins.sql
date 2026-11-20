-- Deployments a person pinned so image retention and cleanup never remove them.
CREATE TABLE deploy_pins (
    deploy_attempt_id TEXT PRIMARY KEY,
    service_name      TEXT NOT NULL,
    pinned_by         TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL
);

CREATE INDEX deploy_pins_service ON deploy_pins (service_name);
