-- App-to-channel attachment, mirroring internal/store's own
-- migrations/0030_service_storage_target.sql precedent. Existing rows
-- keep channel_id NULL and resolve from their own notify_url/notify_kind.
ALTER TABLE deploy_notify_targets ADD COLUMN channel_id TEXT REFERENCES notification_channels(id) ON DELETE SET NULL;
