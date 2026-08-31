-- One row per real notification send attempt (deploy outcome, alert
-- rule, or test-send), so a channel's delivery history reflects whether
-- sends actually worked, not just whether the channel is configured.
CREATE TABLE notification_deliveries (
    id             TEXT PRIMARY KEY,
    channel_id     TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    trigger_reason TEXT NOT NULL,
    success        INTEGER NOT NULL,
    error          TEXT NOT NULL DEFAULT '',

    created_at     TEXT NOT NULL
);

CREATE INDEX idx_notification_deliveries_channel_created ON notification_deliveries(channel_id, created_at DESC);
