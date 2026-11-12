-- Approved template capabilities per service as JSON; empty means none, so existing rows are unchanged.
ALTER TABLE desired_services ADD COLUMN granted_caps TEXT NOT NULL DEFAULT '';
