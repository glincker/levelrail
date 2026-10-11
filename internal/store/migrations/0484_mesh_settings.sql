-- Operator's choice to run the WireGuard mesh, persisted so the dashboard and
-- CLI can turn it on. Absent row means never chosen; APP_MESH_ENABLED wins
-- over this row when set.
CREATE TABLE mesh_settings (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    enabled    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);
