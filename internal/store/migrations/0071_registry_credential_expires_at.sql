-- Optional operator-provided expiry for a registry credential (a GitHub
-- PAT, a cloud registry's short-lived token): the platform cannot detect
-- this from an opaque credential string, so it's set by hand and only
-- used to warn before a deploy's image pull fails on a stale token.

ALTER TABLE registry_credentials ADD COLUMN expires_at TEXT;
