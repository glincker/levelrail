-- Disambiguates which of two containers wrote a line when an old and a
-- new container briefly coexist for the same resource_id during a
-- blue-green rollout cutover. Without this, a live view sorting by ts
-- alone cannot tell a dying old container's final lines (e.g. its
-- shutdown message) apart from the new, current container's own quiet
-- startup. Existing rows predate container identity and get '', not a
-- guess: QueryLogs' live-filtering caller treats '' as "unknown, don't
-- filter out" rather than "not current."
ALTER TABLE log_entries ADD COLUMN container_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_log_entries_resource_container ON log_entries (resource_id, container_id);
