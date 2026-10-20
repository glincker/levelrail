-- First time the orphan reaper saw a leftover resource. The grace period
-- counts from here, so a resource is never reaped on the pass that finds it.
CREATE TABLE orphan_sightings (
    kind          TEXT NOT NULL,
    node_id       TEXT NOT NULL DEFAULT '',
    name          TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    PRIMARY KEY (kind, node_id, name)
);
