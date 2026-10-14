-- One-off scheduled maintenance windows. freeze holds automatic deploys for
-- the window. announced_at is set once the lead-time notice has been sent.
CREATE TABLE maintenance_windows (
    id                  TEXT PRIMARY KEY,
    title               TEXT NOT NULL,
    message             TEXT NOT NULL DEFAULT '',
    scope               TEXT NOT NULL,
    target              TEXT NOT NULL DEFAULT '',
    starts_at           TEXT NOT NULL,
    ends_at             TEXT NOT NULL,
    freeze              INTEGER NOT NULL DEFAULT 0,
    announce_lead_mins  INTEGER NOT NULL DEFAULT 0,
    announced_at        TEXT NOT NULL DEFAULT '',
    created_by          TEXT NOT NULL DEFAULT '',
    created_at          TEXT NOT NULL
);
CREATE INDEX idx_maintenance_windows_ends_at ON maintenance_windows (ends_at);
