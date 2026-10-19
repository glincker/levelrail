CREATE TABLE janitor_policy (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    body       TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT ''
);

CREATE TABLE janitor_runs (
    id              TEXT PRIMARY KEY,
    started_at      TEXT NOT NULL,
    trigger         TEXT NOT NULL,
    dry_run         INTEGER NOT NULL DEFAULT 0,
    reclaimed_bytes INTEGER NOT NULL DEFAULT 0,
    free_percent    REAL NOT NULL DEFAULT -1,
    body            TEXT NOT NULL,
    error           TEXT NOT NULL DEFAULT ''
);

CREATE INDEX janitor_runs_started ON janitor_runs (started_at DESC);

CREATE TABLE janitor_quarantine (
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT -1,
    first_seen TEXT NOT NULL,
    PRIMARY KEY (kind, name)
);
