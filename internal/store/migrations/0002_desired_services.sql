-- Desired runtime state for one service: what the application controller
-- (TASKS.md 1.3) reconciles containers against. Deliberately distinct
-- from internal/spec.Service (the app.yaml declaration): spec.Service
-- describes HOW to build an image, this describes WHICH already-built
-- image should be running right now, plus what it needs to run. A future
-- deploy pipeline (TASKS.md 1.4/1.5) writes rows here once a build
-- produces an image; nothing upstream of that exists yet, so this table
-- is populated directly (by tests, and eventually that pipeline) rather
-- than through any automatic translation from app.yaml.
--
-- env, resources, and health are stored as JSON rather than normalized
-- into columns: they're read and written as whole nested structures, not
-- queried by sub-field, so normalizing them now would be exactly the
-- speculative schema CLAUDE.md 7 argues against.
CREATE TABLE desired_services (
    name       TEXT PRIMARY KEY,
    image      TEXT NOT NULL,
    port       INTEGER NOT NULL,
    env        TEXT NOT NULL DEFAULT '{}',
    resources  TEXT NOT NULL DEFAULT '{}',
    health     TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL
);
