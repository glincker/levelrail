-- A linked user left their team or was disabled while the mail engine was
-- unreachable: the suspend is owed and retried, and the list shows it.
ALTER TABLE mail_mailboxes ADD COLUMN suspend_pending INTEGER NOT NULL DEFAULT 0;
