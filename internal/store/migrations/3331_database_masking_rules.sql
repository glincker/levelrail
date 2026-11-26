-- Versioned masking rules for anonymized branches. Older versions stay read only
-- so a branch can name the exact rules it was built with.
CREATE TABLE database_masking_rulesets (
    id            TEXT PRIMARY KEY,
    database_name TEXT NOT NULL REFERENCES desired_databases(name) ON DELETE CASCADE,
    version       INTEGER NOT NULL,
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    UNIQUE (database_name, version)
);

CREATE TABLE database_masking_rules (
    id            TEXT PRIMARY KEY,
    ruleset_id    TEXT NOT NULL REFERENCES database_masking_rulesets(id) ON DELETE CASCADE,
    target        TEXT NOT NULL CHECK (target != ''),
    kind          TEXT NOT NULL CHECK (kind IN ('null', 'hash', 'partial', 'fake', 'keep')),
    params        TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(params)),
    deterministic INTEGER NOT NULL DEFAULT 0 CHECK (deterministic IN (0, 1)),
    salt_ref      TEXT NOT NULL DEFAULT '',
    position      INTEGER NOT NULL DEFAULT 0,
    CHECK (deterministic = 0 OR kind NOT IN ('hash', 'fake') OR salt_ref != '')
);

CREATE INDEX idx_database_masking_rules_ruleset ON database_masking_rules(ruleset_id, position);
