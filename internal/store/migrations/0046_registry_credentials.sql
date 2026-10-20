-- A registry credential is a username/password (or token-as-password)
-- pair for pulling a private container image, referenced by name from
-- app.yaml's build.type: image (internal/spec.Build.RegistryCredential).
-- No password column here, the same reasoning migrations/0018_backup_
-- targets.sql already established: credentials go through internal/
-- secrets' envelope encryption, keyed by "registry-credential/<id>",
-- never as a plaintext or same-row-encrypted column in this table.
CREATE TABLE registry_credentials (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    registry_host TEXT NOT NULL,
    username      TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

-- Empty string, not NULL, is "no credential referenced": the same
-- convention node_id already uses on this table for "this control
-- plane's own local node" (migrations/0009's own comment). No foreign
-- key: a service referencing a since-deleted credential should fail
-- loudly at pull time (docker.ErrRegistryCredentialNotFound), not be
-- silently cascaded away.
ALTER TABLE desired_services ADD COLUMN registry_credential_id TEXT NOT NULL DEFAULT '';
