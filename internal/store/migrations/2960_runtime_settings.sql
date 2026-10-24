-- Operator-saved runtime settings (security toggles and similar). A row beats
-- the env var, which beats the registry default; deleting the row resets it.
CREATE TABLE runtime_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    actor      TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
