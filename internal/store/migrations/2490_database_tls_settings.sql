-- Operator TLS choice per managed database. No row or mode 'default' keeps the engine default (on for Postgres and Redis, off otherwise).
CREATE TABLE database_tls_settings (
    database_name TEXT PRIMARY KEY,
    mode          TEXT NOT NULL DEFAULT 'default',
    updated_at    TEXT NOT NULL
);
