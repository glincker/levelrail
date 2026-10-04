-- Marks a service deployed via the one-click template "Deploy now" path
-- (POST /api/v1/service-templates/{id}/deploy) as an obviously-temporary
-- trial instance. Insert-only, like app_id: a redeploy never flips it.
ALTER TABLE desired_services ADD COLUMN is_trial INTEGER NOT NULL DEFAULT 0;
