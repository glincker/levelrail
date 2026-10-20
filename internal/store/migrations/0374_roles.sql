-- Roles are named, stored bundles of abilities. The abilities themselves stay
-- the enforcement primitive; a role's visibility says whether its holders see
-- everything or only the environments granted to them.
CREATE TABLE roles (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    abilities   TEXT NOT NULL,
    visibility  TEXT NOT NULL DEFAULT 'all' CHECK (visibility IN ('all', 'granted')),
    builtin     INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

INSERT INTO roles (id, name, description, abilities, visibility, builtin) VALUES
    ('role_admin', 'admin', 'Full control of the instance.', '["root"]', 'all', 1),
    ('role_operator', 'operator', 'Deploy and manage apps, without instance administration.', '["read","read:sensitive","write","deploy"]', 'all', 1),
    ('role_viewer', 'viewer', 'Read-only access to everything that is not sensitive.', '["read"]', 'all', 1),
    ('role_guest', 'guest', 'Read-only access limited to the environments granted to the user.', '["read"]', 'granted', 1);

ALTER TABLE users ADD COLUMN role_id TEXT REFERENCES roles(id) ON DELETE SET NULL;

CREATE TABLE user_environment_grants (
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, environment_id)
);

CREATE INDEX idx_user_environment_grants_environment_id ON user_environment_grants (environment_id);
