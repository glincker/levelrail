-- Second channel for an alert that stays unread: escalate_channel_id receives it once the rule has fired for escalate_after_seconds without an Inbox read.
ALTER TABLE alert_rules ADD COLUMN escalate_channel_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN escalate_after_seconds INTEGER NOT NULL DEFAULT 0;
