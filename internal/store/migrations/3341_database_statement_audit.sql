-- One row per database explorer action. No foreign key: the trail outlives the database.
CREATE TABLE database_statement_audit (
    id             TEXT PRIMARY KEY,
    at             TEXT NOT NULL,
    database_name  TEXT NOT NULL,
    database_id    TEXT NOT NULL DEFAULT '',
    engine         TEXT NOT NULL DEFAULT '',
    principal_type TEXT NOT NULL DEFAULT '',
    principal_id   TEXT NOT NULL DEFAULT '',
    principal_name TEXT NOT NULL DEFAULT '',
    ability        TEXT NOT NULL DEFAULT '',
    action         TEXT NOT NULL CHECK (action IN ('db:read', 'db:query', 'db:write', 'db:export', 'db:admin')),
    op_kind        TEXT NOT NULL DEFAULT '',
    fingerprint    TEXT NOT NULL DEFAULT '',
    statement_text TEXT NOT NULL DEFAULT '',
    rows_returned  INTEGER NOT NULL DEFAULT 0,
    bytes          INTEGER NOT NULL DEFAULT 0,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    outcome        TEXT NOT NULL CHECK (outcome IN ('ok', 'denied', 'rejected', 'timeout', 'cancelled', 'error')),
    error_class    TEXT NOT NULL DEFAULT '',
    client_ip      TEXT NOT NULL DEFAULT '',
    client_kind    TEXT NOT NULL DEFAULT '',
    branch_name    TEXT,
    request_id     TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_database_statement_audit_db_at ON database_statement_audit(database_name, at);
CREATE INDEX idx_database_statement_audit_principal_at ON database_statement_audit(principal_id, at);
CREATE INDEX idx_database_statement_audit_at ON database_statement_audit(at);
