-- The instance's own system email account (noreply) is managed only by root through
-- the system email routes; team mailbox routes refuse it and lists hide it.
ALTER TABLE mail_mailboxes ADD COLUMN system INTEGER NOT NULL DEFAULT 0;
