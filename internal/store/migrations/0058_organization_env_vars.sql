-- Shared env vars at the organization level, one tier above
-- project_env_vars (migrations/0040): the base layer project vars
-- override, mirroring 0040's own "plain values only, no secret support"
-- scope note one level up.
CREATE TABLE organization_env_vars (
    org_id     TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    PRIMARY KEY (org_id, key)
);
