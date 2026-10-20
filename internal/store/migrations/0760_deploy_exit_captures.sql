-- Why a release's container stopped before it could serve, and its last
-- output, captured at the moment the reconciler saw the exit. One row per
-- attempt; attempt_id is '' when no attempt row matched the image.

CREATE TABLE deploy_exit_captures (
    service_name TEXT NOT NULL,
    attempt_id   TEXT NOT NULL DEFAULT '',
    image        TEXT NOT NULL,
    container    TEXT NOT NULL,
    exit_code    INTEGER NOT NULL,
    oom_killed   INTEGER NOT NULL DEFAULT 0,
    reason       TEXT NOT NULL,
    message      TEXT NOT NULL,
    started_at   TEXT NOT NULL DEFAULT '',
    finished_at  TEXT NOT NULL DEFAULT '',
    log_tail     TEXT NOT NULL DEFAULT '[]',
    captured_at  TEXT NOT NULL,
    PRIMARY KEY (service_name, attempt_id)
);

CREATE INDEX idx_deploy_exit_captures_service_captured ON deploy_exit_captures (service_name, captured_at DESC);
