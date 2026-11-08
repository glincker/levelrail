-- Allowlisted docker run options per service as JSON; empty means none, so existing rows are unchanged.
ALTER TABLE desired_services ADD COLUMN runtime_options TEXT NOT NULL DEFAULT '';
