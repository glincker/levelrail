CREATE TABLE IF NOT EXISTS exposure_restrictions (
    port       INTEGER NOT NULL,
    protocol   TEXT NOT NULL,
    allow      TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (port, protocol)
);
