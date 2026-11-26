-- Lineage and lifecycle of a database branch. The branch itself is an ordinary
-- desired_databases row; the row goes with its database so a recreated name starts clean.
-- parent_name has no foreign key: deleting a parent with children is refused in code.
CREATE TABLE database_branches (
    id                     TEXT PRIMARY KEY,
    database_name          TEXT NOT NULL UNIQUE REFERENCES desired_databases(name) ON DELETE CASCADE,
    parent_name            TEXT NOT NULL DEFAULT '',
    root_name              TEXT NOT NULL,
    mode                   TEXT NOT NULL CHECK (mode IN ('full', 'pitr', 'schema', 'anonymized')),
    source                 TEXT NOT NULL CHECK (source IN ('live', 'backup', 'pitr')),
    source_ref             TEXT NOT NULL DEFAULT '',
    clone_path             TEXT NOT NULL DEFAULT 'dump' CHECK (clone_path IN ('dump', 'snapshot')),
    clone_path_reason      TEXT NOT NULL DEFAULT '',
    state                  TEXT NOT NULL DEFAULT 'requested' CHECK (state IN ('requested', 'cloning', 'masking', 'credentials', 'verifying', 'ready', 'resetting', 'promoting', 'promoted', 'expiring', 'deleting', 'deleted', 'failed')),
    state_reason           TEXT NOT NULL DEFAULT '',
    protected              INTEGER NOT NULL DEFAULT 0 CHECK (protected IN (0, 1)),
    expires_at             TEXT NOT NULL DEFAULT '',
    expiry_preset          TEXT NOT NULL DEFAULT '',
    ruleset_id             TEXT NOT NULL DEFAULT '',
    ruleset_version        INTEGER NOT NULL DEFAULT 0,
    preview_environment_id TEXT,
    size_bytes             INTEGER NOT NULL DEFAULT 0,
    size_measured_at       TEXT NOT NULL DEFAULT '',
    last_active_at         TEXT NOT NULL DEFAULT '',
    branched_at            TEXT NOT NULL DEFAULT '',
    created_by             TEXT NOT NULL DEFAULT '',
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL
);

CREATE INDEX idx_database_branches_root ON database_branches(root_name);
CREATE INDEX idx_database_branches_parent ON database_branches(parent_name);
CREATE INDEX idx_database_branches_state_expiry ON database_branches(state, expires_at);
CREATE INDEX idx_database_branches_preview ON database_branches(preview_environment_id);
