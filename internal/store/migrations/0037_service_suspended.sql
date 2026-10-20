-- suspended: an operator-requested "stop" distinct from delete. The
-- desired service row, its image, env, domains, and volumes all stay
-- exactly as they are; only the reconciler's converge target changes to
-- zero running containers. NOT NULL DEFAULT 0 (not nullable), because
-- unlike project_id/storage_target_id there is no third state: a
-- service is either suspended or it isn't.
ALTER TABLE desired_services ADD COLUMN suspended INTEGER NOT NULL DEFAULT 0;
