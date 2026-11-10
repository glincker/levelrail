-- One structured postmortem per incident. The trigger refuses any change to a
-- published row, so the lock holds even for a writer that skips the API guard.
CREATE TABLE incident_postmortems (
    incident_id  TEXT PRIMARY KEY REFERENCES status_incidents(id) ON DELETE CASCADE,
    summary      TEXT NOT NULL DEFAULT '',
    impact       TEXT NOT NULL DEFAULT '',
    root_cause   TEXT NOT NULL DEFAULT '',
    resolution   TEXT NOT NULL DEFAULT '',
    action_items TEXT NOT NULL DEFAULT '[]',
    impact_start TEXT NOT NULL,
    impact_end   TEXT,
    alerts       TEXT NOT NULL DEFAULT '[]',
    state        TEXT NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'published')),
    created_by   TEXT NOT NULL DEFAULT '',
    updated_by   TEXT NOT NULL DEFAULT '',
    published_by TEXT NOT NULL DEFAULT '',
    published_at TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE TRIGGER incident_postmortems_locked BEFORE UPDATE ON incident_postmortems
WHEN OLD.state = 'published'
BEGIN
	SELECT RAISE(ABORT, 'incident postmortem is published');
END;
