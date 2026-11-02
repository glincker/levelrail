-- One id per life of an app name; '' marks rows from before it existed.
ALTER TABLE desired_services ADD COLUMN incarnation TEXT NOT NULL DEFAULT '';
ALTER TABLE pending_teardowns ADD COLUMN incarnation TEXT NOT NULL DEFAULT '';
