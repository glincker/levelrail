-- Signed records of images remote builders produced, linked to the deploy attempt that used them.
CREATE TABLE build_records (
    id         TEXT PRIMARY KEY,
    app_name   TEXT NOT NULL,
    attempt_id TEXT NOT NULL DEFAULT '',
    claims     TEXT NOT NULL,
    signature  TEXT NOT NULL,
    key_id     TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX build_records_attempt ON build_records(attempt_id);
CREATE INDEX build_records_app ON build_records(app_name, created_at);
