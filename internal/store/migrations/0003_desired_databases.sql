-- Desired runtime state for a managed database (CLAUDE.md 6 Phase 1:
-- "Managed Postgres and Redis as first-class resources"), mirroring
-- desired_services' shape and reasoning. Deliberately no credentials
-- column: database auth needs the same envelope-encrypted secret
-- storage CLAUDE.md 4.10 specifies, which doesn't exist yet (TASKS.md
-- 1.7). Adding a plaintext password column now, even as a marked
-- "temporary" stopgap, is exactly the kind of security shortcut this
-- project's own standards rule out. The database controller (1.8)
-- reconciles container and volume existence in this pass; credential
-- wiring is explicitly deferred, not silently skipped.
CREATE TABLE desired_databases (
    name       TEXT PRIMARY KEY,
    engine     TEXT NOT NULL CHECK (engine IN ('postgres', 'redis')),
    version    TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
