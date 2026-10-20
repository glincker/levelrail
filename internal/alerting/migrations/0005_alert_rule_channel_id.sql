-- Rule-to-channel attachment, mirroring migrations/0004's own shape.
-- Existing rows keep channel_id NULL and resolve from their own
-- notify_url/notify_kind.
ALTER TABLE alert_rules ADD COLUMN channel_id TEXT REFERENCES notification_channels(id) ON DELETE SET NULL;
