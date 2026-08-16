-- Public/host-exposed access for managed databases, so an operator can
-- connect a database GUI tool (pgAdmin, TablePlus, RedisInsight) to a
-- managed Postgres/Redis/MySQL from their own machine, not just from
-- inside the Docker network. Off by default.
--
-- publicly_accessible: NOT NULL DEFAULT 0, boolean-as-integer, the same
-- convention this table already uses. public_port: nullable, the host
-- port bound to the database's container port once publicly_accessible
-- is true. NULL means "no port assigned", the same meaning project_id's
-- own NULL already carries; SetDatabasePublicAccess
-- (internal/store/database.go) is the only writer and always sets both
-- columns together.
ALTER TABLE desired_databases ADD COLUMN publicly_accessible INTEGER NOT NULL DEFAULT 0;
ALTER TABLE desired_databases ADD COLUMN public_port INTEGER;

-- Partial unique index: two databases can never be assigned the same
-- host port. This is the hard backstop; SetDatabasePublicAccess checks
-- for a free port before writing, this index is what makes that check
-- race-safe rather than trust-only.
CREATE UNIQUE INDEX idx_desired_databases_public_port ON desired_databases (public_port) WHERE public_port IS NOT NULL;
