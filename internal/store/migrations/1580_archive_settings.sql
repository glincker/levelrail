-- Single-row policy for archiving cold rows into compressed bundles.
-- Hot days: rows older than this leave the live tables. keep_days: how long
-- a bundle is kept before it is deleted (0 keeps forever).
CREATE TABLE archive_settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    enabled             INTEGER NOT NULL DEFAULT 0,
    audit_hot_days      INTEGER NOT NULL DEFAULT 30,
    deploy_hot_days     INTEGER NOT NULL DEFAULT 30,
    pipeline_hot_days   INTEGER NOT NULL DEFAULT 30,
    keep_days           INTEGER NOT NULL DEFAULT 365,
    last_run_at         TEXT NOT NULL DEFAULT '',
    last_run_error      TEXT NOT NULL DEFAULT '',
    updated_at          TEXT NOT NULL DEFAULT ''
);

INSERT INTO archive_settings (id) VALUES (1);
