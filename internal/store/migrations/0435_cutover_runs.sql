-- Guided per-app cutover runs for apps staged by the importer. domains_json
-- holds each domain's previous DNS value so a rollback can restore it.
CREATE TABLE cutover_runs (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    source_id     TEXT NOT NULL,
    app_name      TEXT NOT NULL,
    mode          TEXT NOT NULL CHECK (mode IN ('dry_run', 'switch')),
    state         TEXT NOT NULL CHECK (state IN ('planning', 'ready', 'starting', 'verifying', 'switching', 'live', 'rolled_back', 'failed')),
    method        TEXT NOT NULL DEFAULT '',
    dns_write     INTEGER NOT NULL DEFAULT 0,
    was_routed    INTEGER NOT NULL DEFAULT 0,
    accept_warn   INTEGER NOT NULL DEFAULT 0,
    awaiting      TEXT NOT NULL DEFAULT '',
    domains_json  TEXT NOT NULL DEFAULT '[]',
    steps_json    TEXT NOT NULL DEFAULT '[]',
    plan_json     TEXT NOT NULL DEFAULT '',
    error         TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    finished_at   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX cutover_runs_app ON cutover_runs (app_name, created_at DESC);
CREATE UNIQUE INDEX cutover_runs_one_active ON cutover_runs (app_name)
    WHERE state IN ('planning', 'starting', 'verifying', 'switching');
