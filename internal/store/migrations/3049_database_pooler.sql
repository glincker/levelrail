-- Opt-in transaction pooler child container for a managed Postgres database.
ALTER TABLE desired_databases ADD COLUMN pooler_enabled INTEGER NOT NULL DEFAULT 0;
