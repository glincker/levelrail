CREATE TABLE IF NOT EXISTS database_saved_queries (
    id             TEXT PRIMARY KEY,
    database_name  TEXT NOT NULL,
    principal_type TEXT NOT NULL,
    principal_id   TEXT NOT NULL,
    name           TEXT NOT NULL,
    sql_text       TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    UNIQUE (database_name, principal_type, principal_id, name)
);
