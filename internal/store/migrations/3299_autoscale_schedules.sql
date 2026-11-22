-- Scheduled minimum replica windows for the autoscaler, stored as a JSON array.
ALTER TABLE app_autoscale ADD COLUMN schedules TEXT NOT NULL DEFAULT '[]';
