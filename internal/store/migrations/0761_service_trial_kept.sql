-- When a trial deployment (0264_service_is_trial.sql) was kept as a
-- permanent app, so a repeated keep answers the same instead of 409.

ALTER TABLE desired_services ADD COLUMN trial_kept_at TEXT NOT NULL DEFAULT '';
