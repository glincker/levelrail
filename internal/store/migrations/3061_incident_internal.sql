-- Internal incidents (ADR 061): additive columns so system opened incidents can stay private until
-- an operator publishes them. Defaults keep every existing row public and internally resolved.
ALTER TABLE status_incidents ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE status_incidents ADD COLUMN source_ref TEXT;
ALTER TABLE status_incidents ADD COLUMN internal_state TEXT NOT NULL DEFAULT 'resolved';
ALTER TABLE status_incidents ADD COLUMN published INTEGER NOT NULL DEFAULT 1;
ALTER TABLE status_incidents ADD COLUMN published_at TEXT;
ALTER TABLE status_incidents ADD COLUMN published_by TEXT;
ALTER TABLE status_incidents ADD COLUMN acknowledged_at TEXT;
ALTER TABLE status_incidents ADD COLUMN acknowledged_by TEXT;
ALTER TABLE status_incidents ADD COLUMN internal_resolved_at TEXT;
ALTER TABLE status_incidents ADD COLUMN draft_title TEXT;
ALTER TABLE status_incidents ADD COLUMN draft_body TEXT;
ALTER TABLE status_incident_updates ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public';
ALTER TABLE status_incident_updates ADD COLUMN author TEXT;
CREATE UNIQUE INDEX idx_status_incidents_open_source
    ON status_incidents (source, source_ref)
    WHERE internal_state != 'resolved' AND source_ref IS NOT NULL;
CREATE TABLE status_incident_links (
    incident_id TEXT NOT NULL REFERENCES status_incidents(id) ON DELETE CASCADE,
    monitor_id  TEXT NOT NULL,
    down_at     TEXT NOT NULL,
    up_at       TEXT,
    PRIMARY KEY (incident_id, monitor_id, down_at)
);
