-- Which network interface a managed database's public port
-- (migrations/0026_database_public_access.sql) binds to
-- (internal/bindaddr): "private", "public", or a literal IP. NULL when
-- publicly_accessible is false, the same meaning public_port's own NULL
-- already carries; SetDatabasePublicAccess is the only writer.
--
-- Every database already publicly accessible at migration time is
-- backfilled to 'public' below, preserving the exposure it already
-- relied on. A database made public after this ships defaults to
-- 'private' at the Go layer (store.claimPublicPort's caller,
-- database.go) when no bind address is requested, the same "safer
-- default going forward, no silent regression for what's already
-- running" compromise migrations/0098_service_bind_address.sql applies
-- to application services.
ALTER TABLE desired_databases ADD COLUMN public_bind_address TEXT;
UPDATE desired_databases SET public_bind_address = 'public' WHERE publicly_accessible = 1;
