-- Reusable incident post presets so operators fill title, status, impact and body in one pick.
CREATE TABLE status_incident_templates (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    title      TEXT NOT NULL,
    status     TEXT NOT NULL,
    impact     TEXT NOT NULL,
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
