-- 0003_desired_databases.sql's engine column carries a hardcoded
-- CHECK (engine IN ('postgres', 'redis')), the exact class of "adding an
-- engine means hunting down every place its name is hardcoded" problem
-- database_engines.yaml (internal/store/database_engines.go) now exists
-- specifically to solve: engine validity is enforced at the application
-- layer (validateDatabaseResource, internal/api/databases.go) against
-- that dynamic registry, so a database-level CHECK duplicating the same
-- list, and needing its own migration every time the registry gains an
-- engine, is no longer the right place to enforce it. Dropped here
-- rather than updated to add 'mysql': updating it would just move the
-- same hardcoding problem one migration down the road instead of
-- removing it.
--
-- SQLite has no ALTER TABLE ... DROP CONSTRAINT, so this is the standard
-- recreate-and-copy pattern: no other table has a foreign key into
-- desired_databases (confirmed via grep across every migration file), so
-- this is a straight copy, not a multi-table rebuild. Column set mirrors
-- the table's full current shape, not just 0003's original one:
-- 0009_node_placement.sql later added node_id via ALTER TABLE ADD
-- COLUMN, which a recreate must carry forward or silently drop it.
CREATE TABLE desired_databases_new (
    name       TEXT PRIMARY KEY,
    engine     TEXT NOT NULL,
    version    TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    node_id    TEXT NOT NULL DEFAULT ''
);

INSERT INTO desired_databases_new (name, engine, version, updated_at, node_id)
SELECT name, engine, version, updated_at, node_id FROM desired_databases;

DROP TABLE desired_databases;

ALTER TABLE desired_databases_new RENAME TO desired_databases;
