-- Runbook is a URL or short note shown in the notification and the Inbox item so the responder knows what to do.
ALTER TABLE alert_rules ADD COLUMN runbook TEXT NOT NULL DEFAULT '';
