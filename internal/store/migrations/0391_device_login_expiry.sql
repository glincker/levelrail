ALTER TABLE audit_log ADD COLUMN action TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_audit_log_action ON audit_log (action, created_at DESC);

CREATE TABLE device_login_expiries (
    request_id TEXT PRIMARY KEY,
    expired_at TEXT NOT NULL,
    audit_id   TEXT NOT NULL
);
