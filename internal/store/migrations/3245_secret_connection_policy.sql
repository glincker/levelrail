-- Per connection allow lists (path prefixes and environments) as JSON; '{}' means none.
ALTER TABLE secret_connections ADD COLUMN policy TEXT NOT NULL DEFAULT '{}';
