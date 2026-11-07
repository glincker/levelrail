-- Reusable node bootstrap scripts appended to the generated first-boot script of a new cloud node.
CREATE TABLE bootstrap_scripts (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    body       TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Runtime overrides for the size and count limits; NULL means the env seed applies.
CREATE TABLE bootstrap_script_limits (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    max_bytes INTEGER,
    max_count INTEGER
);
INSERT INTO bootstrap_script_limits (id) VALUES (1);
