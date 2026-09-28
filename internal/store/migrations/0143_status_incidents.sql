-- Operator-authored incidents and scheduled maintenance announcements.
-- Bodies are markdown-lite text, escaped at render time.
CREATE TABLE status_incidents (
    id             TEXT PRIMARY KEY,
    kind           TEXT NOT NULL,   -- incident | maintenance
    title          TEXT NOT NULL,
    status         TEXT NOT NULL,   -- investigating | identified | monitoring | resolved | scheduled | in_progress | completed
    impact         TEXT NOT NULL DEFAULT 'none',   -- none | minor | major | critical
    component_ids  TEXT NOT NULL DEFAULT '[]',
    starts_at      TEXT NOT NULL,
    ends_at        TEXT,
    resolved_at    TEXT,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE INDEX idx_status_incidents_starts ON status_incidents (starts_at DESC);

CREATE TABLE status_incident_updates (
    id          TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES status_incidents(id) ON DELETE CASCADE,
    status      TEXT NOT NULL,
    body        TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE INDEX idx_status_incident_updates_incident ON status_incident_updates (incident_id, created_at);
