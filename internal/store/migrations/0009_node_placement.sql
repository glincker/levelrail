-- Phase 3 (TASKS.md 3.3): placement. Every service and database has
-- implicitly run on this control plane's own local Docker daemon since
-- Phase 1; this migration is what first lets one be pointed anywhere
-- else. An empty string is the explicit "local node" value, not NULL
-- and not a foreign key to a "local" row in the nodes table (0008):
-- every service/database that predates this migration gets '' via
-- DEFAULT, which is exactly the local-node behavior it already had,
-- zero behavior change for a single-node deployment on upgrade. There
-- is deliberately no FOREIGN KEY to nodes.id either: node_id is only
-- ever meaningful when non-empty, at which point cmd/levelrail's own
-- dynamicSource (TASKS.md 3.3) resolves it through
-- internal/agent.Registry, which already has its own "not currently
-- registered" error path for a node that's since been removed or
-- hasn't connected yet; a foreign key would turn that into an
-- unrecoverable constraint violation instead of a normal, transient
-- "not connected right now" state.
ALTER TABLE desired_services ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
