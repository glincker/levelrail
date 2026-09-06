-- Optional alternate Postgres image (pgvector, PostGIS, TimescaleDB
-- variants; see database_engines.yaml's postgres.variants list). Empty
-- means the vanilla image, unchanged behavior for every existing row.
ALTER TABLE desired_databases ADD COLUMN variant TEXT NOT NULL DEFAULT '';
