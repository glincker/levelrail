-- Per-app cutover checklist for apps moved off another platform. steps_json
-- holds each step's state, evidence and who moved it.
CREATE TABLE migration_checklists (
    app_name   TEXT PRIMARY KEY,
    platform   TEXT NOT NULL DEFAULT '',
    source_id  TEXT NOT NULL DEFAULT '',
    steps_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
