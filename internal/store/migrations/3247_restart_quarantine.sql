-- Containers the crash-loop breaker holds, so a control plane restart does not release them.
CREATE TABLE restart_quarantine (
    container       TEXT PRIMARY KEY,
    app_name        TEXT NOT NULL DEFAULT '',
    restarts        INTEGER NOT NULL DEFAULT 0,
    quarantined_at  TEXT NOT NULL
);
