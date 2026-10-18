-- Measured chaos scenario runs, shown read-only as the resilience report.
CREATE TABLE chaos_results (
    id              TEXT PRIMARY KEY,
    scenario        TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('passed', 'failed', 'skipped')),
    summary         TEXT NOT NULL DEFAULT '',
    recovery_ms     REAL NOT NULL DEFAULT 0,
    error_window_ms REAL NOT NULL DEFAULT 0,
    requests        INTEGER NOT NULL DEFAULT 0,
    request_errors  INTEGER NOT NULL DEFAULT 0,
    assertions      TEXT NOT NULL DEFAULT '[]',
    hardware        TEXT NOT NULL DEFAULT '',
    duration_ms     REAL NOT NULL DEFAULT 0,
    ran_at          TEXT NOT NULL,
    recorded_by     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX chaos_results_scenario_ran ON chaos_results (scenario, ran_at DESC);
