-- pull_policy: "always" forces a fresh image pull at deploy time even
-- when the tag already exists locally, empty (the default) keeps
-- today's pull-if-absent behavior. Plain string, not JSON, same shape
-- registry_credential_id already uses on this table (0046). Populated
-- from a compose service's pull_policy: (internal/compose).
ALTER TABLE desired_services ADD COLUMN pull_policy TEXT NOT NULL DEFAULT '';
