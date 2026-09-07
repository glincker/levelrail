-- Per-database init scripts: named SQL/shell files an operator attaches
-- to a managed database, mounted read-only into the container's
-- /docker-entrypoint-initdb.d (internal/reconcile/database's own
-- reconcileEngine), the directory the official postgres/mysql/mariadb/
-- mongo images already auto-execute, in filename-sorted order, the
-- first time (and only the first time) their container starts against
-- an empty data volume. A child table of desired_databases, the same
-- "one row per child resource, ON DELETE CASCADE" shape scheduled_tasks
-- (0048_scheduled_tasks.sql) already establishes for desired_services,
-- rather than a JSON column: any number of independently named/edited
-- scripts, not a single nested blob.
--
-- content is plaintext, not envelope-encrypted like internal/secrets'
-- per-service values: this is DDL/setup code an operator writes and
-- expects to read back and edit, not a credential this platform
-- generates and only ever injects. A script that needs a real secret
-- should read it from an env var the container already has (the same
-- database's own generated credentials), not embed one literally; the
-- dashboard says so.
CREATE TABLE database_init_scripts (
    id            TEXT PRIMARY KEY,
    database_name TEXT NOT NULL REFERENCES desired_databases(name) ON DELETE CASCADE,
    filename      TEXT NOT NULL,
    content       TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

-- Execution order is Docker's own filename sort, not a column here: an
-- operator names files "01-extensions.sql", "02-roles.sql" the same way
-- they would on a real filesystem, so this table's own row order never
-- needs to claim authority over something it doesn't control.
CREATE UNIQUE INDEX ux_database_init_scripts_database_filename ON database_init_scripts(database_name, filename);
CREATE INDEX idx_database_init_scripts_database_name ON database_init_scripts(database_name);
