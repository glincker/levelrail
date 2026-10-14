-- 0010: account lifecycle flags (forced password change, disable) and
-- remember-this-device records.
CREATE TABLE IF NOT EXISTS theauth_account_states (
    user_id              TEXT PRIMARY KEY REFERENCES theauth_users (id) ON DELETE CASCADE,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    disabled_at          INTEGER
);

CREATE TABLE IF NOT EXISTS theauth_trusted_devices (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    token_hash   BLOB NOT NULL,
    label        TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    ip           TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL,
    last_used_at INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    revoked_at   INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_trusted_devices_hash_uq ON theauth_trusted_devices (token_hash);
CREATE INDEX IF NOT EXISTS theauth_trusted_devices_user_idx ON theauth_trusted_devices (user_id);

-- 0011: constrained device identity. A device is registered by redeeming a
-- single-use join token and then authenticates with an RFC 7523 assertion
-- signed by its enrolled key. Join tokens are stored only as SHA-256 digests.
-- No foreign keys on organization_id or created_by: rows may outlive them.
CREATE TABLE IF NOT EXISTS theauth_iot_devices (
    id              TEXT PRIMARY KEY,
    organization_id TEXT,
    name            TEXT NOT NULL DEFAULT '',
    spiffe_id       TEXT,
    public_key_jwk  TEXT NOT NULL,
    key_thumbprint  TEXT NOT NULL,
    attestation     TEXT NOT NULL DEFAULT 'join_token',
    scopes          TEXT NOT NULL DEFAULT '[]',
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      INTEGER NOT NULL,
    last_seen_at    INTEGER,
    revoked_at      INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_iot_devices_spiffe_uq ON theauth_iot_devices (spiffe_id);
CREATE INDEX IF NOT EXISTS theauth_iot_devices_org_idx ON theauth_iot_devices (organization_id, created_at DESC);

CREATE TABLE IF NOT EXISTS theauth_device_join_tokens (
    id              TEXT PRIMARY KEY,
    token_hash      BLOB NOT NULL,
    token_prefix    TEXT NOT NULL DEFAULT '',
    organization_id TEXT,
    name            TEXT NOT NULL DEFAULT '',
    spiffe_id       TEXT NOT NULL DEFAULT '',
    scopes          TEXT NOT NULL DEFAULT '[]',
    created_by      TEXT,
    created_at      INTEGER NOT NULL,
    expires_at      INTEGER NOT NULL,
    redeemed_at     INTEGER,
    device_id       TEXT,
    revoked_at      INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_device_join_tokens_hash_uq ON theauth_device_join_tokens (token_hash);
CREATE INDEX IF NOT EXISTS theauth_device_join_tokens_org_idx ON theauth_device_join_tokens (organization_id, created_at DESC);

-- 0012: OpenID Shared Signals Framework transmitter state. A stream is one
-- receiver subscription (the full document is kept as JSON), subjects are
-- the optional per-stream subject filter, and the queue holds signed SETs
-- awaiting push delivery or poll acknowledgement. No foreign keys: rows are
-- removed explicitly when a stream is deleted.
CREATE TABLE IF NOT EXISTS theauth_signal_streams (
    id          TEXT PRIMARY KEY,
    receiver_id TEXT NOT NULL,
    doc         TEXT NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS theauth_signal_streams_receiver_idx ON theauth_signal_streams (receiver_id, created_at);

CREATE TABLE IF NOT EXISTS theauth_signal_subjects (
    stream_id   TEXT NOT NULL,
    subject_key TEXT NOT NULL,
    subject     TEXT NOT NULL,
    PRIMARY KEY (stream_id, subject_key)
);

CREATE TABLE IF NOT EXISTS theauth_signal_queue (
    id         TEXT PRIMARY KEY,
    stream_id  TEXT NOT NULL,
    jti        TEXT NOT NULL,
    token      TEXT NOT NULL,
    push       INTEGER NOT NULL DEFAULT 0,
    attempts   INTEGER NOT NULL DEFAULT 0,
    next_at    INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS theauth_signal_queue_stream_idx ON theauth_signal_queue (stream_id, created_at);
CREATE INDEX IF NOT EXISTS theauth_signal_queue_due_idx ON theauth_signal_queue (push, next_at);

-- 0013: organizations, membership and the per-session active organization.
-- The active organization lives in its own table so the core sessions table
-- keeps its shape and hosts that never enable organizations are untouched.
CREATE TABLE IF NOT EXISTS theauth_organizations (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL COLLATE NOCASE,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_organizations_slug_uq ON theauth_organizations (slug);

CREATE TABLE IF NOT EXISTS theauth_organization_members (
    organization_id TEXT NOT NULL REFERENCES theauth_organizations (id) ON DELETE CASCADE,
    user_id         TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member')),
    joined_at       INTEGER NOT NULL,
    PRIMARY KEY (organization_id, user_id)
);

CREATE INDEX IF NOT EXISTS theauth_organization_members_user_idx ON theauth_organization_members (user_id);

CREATE TABLE IF NOT EXISTS theauth_session_organizations (
    session_id      TEXT PRIMARY KEY REFERENCES theauth_sessions (id) ON DELETE CASCADE,
    organization_id TEXT NOT NULL REFERENCES theauth_organizations (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS theauth_session_organizations_org_idx ON theauth_session_organizations (organization_id);

-- 0014: SAML connections (one per organization and IdP) and the
-- (connection, NameID) to user identity map.
CREATE TABLE IF NOT EXISTS theauth_saml_connections (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES theauth_organizations (id) ON DELETE CASCADE,
    idp_entity_id   TEXT NOT NULL,
    idp_sso_url     TEXT NOT NULL,
    idp_x509_cert   TEXT NOT NULL,
    sp_entity_id    TEXT NOT NULL,
    sp_acs_url      TEXT NOT NULL,
    attribute_map   TEXT NOT NULL DEFAULT '{}',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS theauth_saml_connections_org_idx ON theauth_saml_connections (organization_id);

CREATE TABLE IF NOT EXISTS theauth_saml_identities (
    id             TEXT PRIMARY KEY,
    connection_id  TEXT NOT NULL REFERENCES theauth_saml_connections (id) ON DELETE CASCADE,
    user_id        TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    name_id        TEXT NOT NULL,
    name_id_format TEXT NOT NULL DEFAULT '',
    last_login_at  INTEGER,
    created_at     INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_saml_identities_conn_name_uq ON theauth_saml_identities (connection_id, name_id);
CREATE INDEX IF NOT EXISTS theauth_saml_identities_user_idx ON theauth_saml_identities (user_id);

-- 0015: SCIM bearer tokens (sha256 hash only), groups and group membership.
-- Requires 0013. The SCIM user columns (external_id and the name parts)
-- already exist on theauth_users from 0001.
CREATE TABLE IF NOT EXISTS theauth_scim_tokens (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES theauth_organizations (id) ON DELETE CASCADE,
    token_hash      BLOB NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    last_used_at    INTEGER,
    revoked_at      INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_scim_tokens_hash_uq ON theauth_scim_tokens (token_hash);
CREATE INDEX IF NOT EXISTS theauth_scim_tokens_org_idx ON theauth_scim_tokens (organization_id);

CREATE TABLE IF NOT EXISTS theauth_groups (
    id              TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES theauth_organizations (id) ON DELETE CASCADE,
    display_name    TEXT NOT NULL,
    external_id     TEXT,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_groups_org_name_uq ON theauth_groups (organization_id, display_name);
CREATE UNIQUE INDEX IF NOT EXISTS theauth_groups_org_external_uq ON theauth_groups (organization_id, external_id)
    WHERE external_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS theauth_group_members (
    group_id TEXT NOT NULL REFERENCES theauth_groups (id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS theauth_group_members_user_idx ON theauth_group_members (user_id);

-- 0016: RBAC. Permissions are a global catalog; roles are organization
-- scoped, with a NULL organization meaning a system role. Requires 0013.
CREATE TABLE IF NOT EXISTS theauth_permissions (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS theauth_permissions_name_uq ON theauth_permissions (name);

CREATE TABLE IF NOT EXISTS theauth_roles (
    id              TEXT PRIMARY KEY,
    organization_id TEXT REFERENCES theauth_organizations (id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- NULL organizations never collide under a plain UNIQUE, so system roles get their own partial index.
CREATE UNIQUE INDEX IF NOT EXISTS theauth_roles_org_name_uq ON theauth_roles (organization_id, name)
    WHERE organization_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS theauth_roles_system_name_uq ON theauth_roles (name)
    WHERE organization_id IS NULL;

CREATE TABLE IF NOT EXISTS theauth_role_permissions (
    role_id       TEXT NOT NULL REFERENCES theauth_roles (id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES theauth_permissions (id) ON DELETE RESTRICT,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS theauth_user_roles (
    user_id    TEXT NOT NULL REFERENCES theauth_users (id) ON DELETE CASCADE,
    role_id    TEXT NOT NULL REFERENCES theauth_roles (id) ON DELETE CASCADE,
    granted_at INTEGER NOT NULL,
    granted_by TEXT REFERENCES theauth_users (id) ON DELETE SET NULL,
    PRIMARY KEY (user_id, role_id)
);

CREATE INDEX IF NOT EXISTS theauth_user_roles_role_idx ON theauth_user_roles (role_id);

