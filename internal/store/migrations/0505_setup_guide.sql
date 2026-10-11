-- Get started hub: per-instance dismissal and per-step operator overrides.
-- Step state itself is derived live from real resources and never stored.
CREATE TABLE setup_guide_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    hidden     INTEGER NOT NULL DEFAULT 0,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
);

INSERT INTO setup_guide_settings (id) VALUES (1);

CREATE TABLE setup_guide_step_overrides (
    step_id    TEXT PRIMARY KEY,
    state      TEXT NOT NULL CHECK (state IN ('skipped', 'done')),
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
);
