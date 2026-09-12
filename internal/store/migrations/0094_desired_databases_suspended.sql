-- suspended: DesiredDatabase's counterpart to desired_services.suspended
-- (0037_service_suspended.sql), same reasoning: an operator-requested
-- stop, the data volume and every other column untouched, only the
-- reconciler's converge target changes to zero running containers.
ALTER TABLE desired_databases ADD COLUMN suspended INTEGER NOT NULL DEFAULT 0;
