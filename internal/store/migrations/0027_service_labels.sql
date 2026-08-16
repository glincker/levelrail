-- Custom Docker labels an operator wants applied to a service's
-- container, internal/spec.Service.Labels' storage home. Stored as JSON
-- in a TEXT column rather than a normalized table, matching the env
-- column's own reasoning (0002_desired_services.sql): read and written
-- as one whole map, never queried by individual key, so normalizing it
-- would be speculative schema for a query pattern that doesn't exist.
ALTER TABLE desired_services ADD COLUMN labels TEXT NOT NULL DEFAULT '{}';
