-- Webhook registration outcome and poll bookkeeping per git source, so a missed webhook is visible and recoverable.
CREATE TABLE git_source_reliability (
    service_name          TEXT PRIMARY KEY REFERENCES service_git_sources(service_name) ON DELETE CASCADE,
    webhook_state         TEXT NOT NULL DEFAULT '',
    webhook_error         TEXT NOT NULL DEFAULT '',
    webhook_changed_at    TEXT NOT NULL DEFAULT '',
    last_webhook_at       TEXT NOT NULL DEFAULT '',
    last_seen_head        TEXT NOT NULL DEFAULT '',
    last_seen_at          TEXT NOT NULL DEFAULT '',
    last_polled_at        TEXT NOT NULL DEFAULT '',
    poll_error            TEXT NOT NULL DEFAULT ''
);
