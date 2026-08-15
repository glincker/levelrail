-- Lightweight, non-auth organizational grouping (repo plan section 6,
-- Phase 4 note "resist the urge to build a permission matrix": this is
-- deliberately not that. No teams, no per-project permissions, the
-- single admin user still sees every project and every app/database
-- regardless of project membership, exactly as it already does today).
-- A project is purely a label an app or database can optionally belong
-- to, the way Coolify groups a web app plus its Postgres plus its Redis
-- under one named project for organization, well before any team or
-- role concept exists.
--
-- id is TEXT, minted by internal/api the same "opaque, mint-time-random
-- identifier" way randomBackupTargetID/randomTokenID already mint theirs
-- (crypto/rand bytes, base64 URL encoding, a short prefix), not an
-- INTEGER AUTOINCREMENT: consistent with every other user-facing
-- resource id in this schema (backup_targets.id, nodes.id, api_tokens.id).
--
-- name has no UNIQUE constraint on purpose: unlike an app or database
-- name (which is also how that resource is addressed in its own URL
-- path, GET /api/v1/apps/{name}), a project's name is a pure label, an
-- id is always how it's addressed (GET /api/v1/projects/{id}), so two
-- projects sharing a display name is a harmless UI quirk, not an
-- addressing collision the way a duplicate app name would be. Adding a
-- uniqueness check here would be exactly the kind of permission-matrix-
-- adjacent gold-plating the Phase 4 scope note warns against for a
-- feature explicitly scoped as "purely a labeling/filtering concept".
CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- project_id on both resource tables: nullable (unlike node_id, which
-- is NOT NULL DEFAULT '' because "" is itself a meaningful value, this
-- control plane's own local node). A project is opt-in, real,
-- permanent state either way: NULL genuinely means "not grouped into
-- any project", not "unset" or "defaulted", and every existing app and
-- database gets NULL on upgrade, which is exactly its current behavior
-- (no project concept existed before this migration), zero behavior
-- change for an upgrading single-node deployment.
--
-- REFERENCES projects(id) ON DELETE SET NULL, unlike node_id's own
-- migration comment (0009_node_placement.sql) explicitly rejecting a
-- foreign key for node_id: that rejection is specific to node_id
-- pointing at a *remote, possibly-disconnected* resource resolved
-- through internal/agent.Registry, where a hard constraint would turn a
-- normal "not connected right now" transient state into an
-- unrecoverable violation. A project has no such transient-availability
-- concern, it is always a plain local row in this same database, so a
-- real foreign key is exactly the right tool here: it is what makes
-- "deleting a project doesn't delete its resources, they just become
-- project-less again" an enforced database guarantee rather than an
-- application-layer convention every future caller has to remember to
-- honor. This is a deliberate difference from node_id's own migration,
-- not an oversight; see that file's comment for the reasoning this one
-- is choosing not to repeat.
ALTER TABLE desired_services ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE SET NULL;
ALTER TABLE desired_databases ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE SET NULL;

-- Supports the project detail page's per-project filtering (today done
-- against the same in-memory list the unfiltered /apps and /databases
-- pages already fetch, see internal/api/projects.go's own doc comment
-- for why there is no server-side ListDesiredServicesByProject yet) and
-- a future one if that changes; harmless and cheap to have now either
-- way, the same "index the column your own delete-guard/list query
-- shape would use" reasoning idx_deploy_attempts_service_started
-- (0021_deploy_attempts.sql) already applies.
CREATE INDEX idx_desired_services_project_id ON desired_services (project_id);
CREATE INDEX idx_desired_databases_project_id ON desired_databases (project_id);
