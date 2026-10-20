-- env_dirty tracks a service whose env/secret-env names were saved via
-- the ordinary app update endpoint (PUT /api/v1/apps/{name}) since its
-- last real container recreation: env vars are baked into a container at
-- create time, so a save here is silently not live on the running
-- container until a restart or redeploy picks it up.
ALTER TABLE desired_services ADD COLUMN env_dirty INTEGER NOT NULL DEFAULT 0;
