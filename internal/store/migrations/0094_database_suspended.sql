-- suspended: the database counterpart to migrations/0037_service_suspended.sql:
-- an operator-requested "stop" distinct from delete. The desired database
-- row, its engine, version, and data volume all stay exactly as they
-- are; only the reconciler's converge target changes to zero running
-- containers. NOT NULL DEFAULT 0, same "no third state" reasoning
-- 0037 already gives.
ALTER TABLE desired_databases ADD COLUMN suspended INTEGER NOT NULL DEFAULT 0;
