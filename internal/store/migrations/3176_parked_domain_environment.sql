-- A domain parked from an environment-scoped set remembers that environment
-- (its ID) so the list can show where it came from. Empty means the app's
-- default domain list.
ALTER TABLE parked_domains ADD COLUMN parked_from_environment TEXT NOT NULL DEFAULT '';
