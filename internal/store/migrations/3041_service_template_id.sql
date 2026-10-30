-- Records which catalog template a service was deployed from so the
-- dashboard can show its setup notes. Insert-only: a PUT never rewrites it.
ALTER TABLE desired_services ADD COLUMN template_id TEXT NOT NULL DEFAULT '';
