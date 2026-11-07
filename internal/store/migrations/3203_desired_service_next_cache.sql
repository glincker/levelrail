-- Per app override of app.yaml next.cache (none or redis); empty defers to the spec.
ALTER TABLE desired_services ADD COLUMN next_cache TEXT NOT NULL DEFAULT '';
