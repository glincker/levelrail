-- Per-app post-deploy watch mode (off, notify, auto) and the soft latency
-- signal overrides. NULL inherits the APP_DEPLOY_WATCH_* default; the mode
-- list is code, not a CHECK.
ALTER TABLE deploy_safety_settings ADD COLUMN watch_mode TEXT;
ALTER TABLE deploy_safety_settings ADD COLUMN watch_latency_ratio REAL CHECK (watch_latency_ratio IS NULL OR watch_latency_ratio >= 0);
ALTER TABLE deploy_safety_settings ADD COLUMN watch_soft_rollback INTEGER CHECK (watch_soft_rollback IS NULL OR watch_soft_rollback IN (0, 1));
