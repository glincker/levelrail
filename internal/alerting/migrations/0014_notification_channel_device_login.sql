-- notify_device_login opts a channel into a link-only notice when a CLI
-- device login is waiting for approval. Off by default.
ALTER TABLE notification_channels ADD COLUMN notify_device_login INTEGER NOT NULL DEFAULT 0;
