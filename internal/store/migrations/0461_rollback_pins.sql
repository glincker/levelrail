-- Images kept tagged as rollback targets so dangling-image cleanup can never
-- remove them. One row per pinned succeeded deploy attempt.
CREATE TABLE rollback_pins (
    service_name      TEXT NOT NULL,
    deploy_attempt_id TEXT NOT NULL,
    image             TEXT NOT NULL,
    image_id          TEXT NOT NULL,
    pin_ref           TEXT NOT NULL,
    created_at        TEXT NOT NULL,
    PRIMARY KEY (service_name, deploy_attempt_id)
);

CREATE INDEX rollback_pins_image_id ON rollback_pins (image_id);
