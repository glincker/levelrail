-- Benchmark results files uploaded from `bench chaos run --out`, kept whole so a run can be shown and compared later.
CREATE TABLE benchmark_runs (
    id           TEXT PRIMARY KEY,
    generated_at TEXT NOT NULL,
    version      TEXT NOT NULL,
    commit_sha   TEXT NOT NULL,
    dirty        INTEGER NOT NULL DEFAULT 0,
    host_summary TEXT NOT NULL,
    host_os      TEXT NOT NULL,
    repeat       INTEGER NOT NULL DEFAULT 1,
    passed       INTEGER NOT NULL DEFAULT 0,
    failed       INTEGER NOT NULL DEFAULT 0,
    skipped      INTEGER NOT NULL DEFAULT 0,
    report       TEXT NOT NULL,
    recorded_by  TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL
);

CREATE INDEX benchmark_runs_generated ON benchmark_runs (generated_at DESC);
