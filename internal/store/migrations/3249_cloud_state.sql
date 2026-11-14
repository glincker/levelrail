-- Cloud provisioning state per account and stack, held sealed, with a DB lock that hands out
-- a monotonic fencing token so a writer that lost its lease can never overwrite newer state.
CREATE TABLE cloud_states (
    account     TEXT NOT NULL,
    stack       TEXT NOT NULL,
    version     INTEGER NOT NULL,
    fence       INTEGER NOT NULL,
    sealed      BLOB NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (account, stack)
);

-- One row per account and stack that was ever locked; an empty holder means free.
-- The row is never deleted so the fencing token only grows.
CREATE TABLE cloud_state_locks (
    account      TEXT NOT NULL,
    stack        TEXT NOT NULL,
    holder       TEXT NOT NULL DEFAULT '',
    fence        INTEGER NOT NULL DEFAULT 0,
    acquired_at  TEXT NOT NULL DEFAULT '',
    expires_at   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (account, stack)
);

-- Steals of an expired lock, kept apart from the audit log so the engine can record them without an actor.
CREATE TABLE cloud_state_lock_steals (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    account          TEXT NOT NULL,
    stack            TEXT NOT NULL,
    previous_holder  TEXT NOT NULL,
    previous_fence   INTEGER NOT NULL,
    new_holder       TEXT NOT NULL,
    new_fence        INTEGER NOT NULL,
    stolen_at        TEXT NOT NULL
);
CREATE INDEX cloud_state_lock_steals_stack_idx ON cloud_state_lock_steals (account, stack);
