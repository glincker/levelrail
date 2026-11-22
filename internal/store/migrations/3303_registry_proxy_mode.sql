-- Optional pull-through cache mode for the built-in registry: one upstream per instance.
-- The upstream password goes through internal/secrets, never a column.
ALTER TABLE registry_settings ADD COLUMN proxy_remote_url TEXT;
ALTER TABLE registry_settings ADD COLUMN proxy_username TEXT;
