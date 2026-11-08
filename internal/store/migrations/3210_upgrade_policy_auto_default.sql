-- Automatic upgrade policy: timed pause, one-time default-on notice dismissal and the post-upgrade health window.
ALTER TABLE upgrade_policy ADD COLUMN paused_until TEXT NOT NULL DEFAULT '';
ALTER TABLE upgrade_policy ADD COLUMN notice_dismissed_at TEXT NOT NULL DEFAULT '';
ALTER TABLE upgrade_policy ADD COLUMN health_window_seconds INTEGER NOT NULL DEFAULT 0;
