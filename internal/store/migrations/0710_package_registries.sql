-- Private package registry credentials injected into builds as secret
-- mounts. The token itself lives in the secrets store, never in this row.

CREATE TABLE package_registries (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    ecosystem       TEXT NOT NULL CHECK (ecosystem IN ('npm', 'pypi', 'maven', 'nuget', 'cargo', 'go', 'rubygems')),
    url             TEXT NOT NULL,
    username        TEXT NOT NULL DEFAULT '',
    module_patterns TEXT NOT NULL DEFAULT '',
    scope_kind      TEXT NOT NULL DEFAULT 'instance' CHECK (scope_kind IN ('instance', 'project', 'app')),
    scope_ref       TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL
);
