-- preview_env_overrides: JSON object, envVarName -> override value,
-- applied on top of the parent app's own env only when a preview
-- environment is created from it (internal/api/preview_environments.go's
-- deployPreviewSingle), never affecting the parent app's own deploys.
-- Same "dedicated setter, excluded from SaveDesiredService's full-record-
-- replace" treatment as log_drain (migrations/0047): an ordinary app.yaml
-- redeploy must never silently wipe a preview override.
ALTER TABLE desired_services ADD COLUMN preview_env_overrides TEXT NOT NULL DEFAULT '{}';
