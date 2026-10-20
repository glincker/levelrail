CREATE TABLE IF NOT EXISTS database_network_rule_notes (
    database_name TEXT NOT NULL,
    source        TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (database_name, source)
);
