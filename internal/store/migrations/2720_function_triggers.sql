-- Function triggers: a signed webhook per function and a durable async invocation queue with a dead-letter state.
CREATE TABLE function_webhooks (
    function   TEXT PRIMARY KEY,
    path       TEXT NOT NULL DEFAULT '/',
    created_at TEXT NOT NULL,
    rotated_at TEXT
);

CREATE TABLE function_invocations (
    id               TEXT PRIMARY KEY,
    function         TEXT NOT NULL,
    source           TEXT NOT NULL,
    method           TEXT NOT NULL DEFAULT 'POST',
    path             TEXT NOT NULL DEFAULT '/',
    content_type     TEXT NOT NULL DEFAULT '',
    body             BLOB,
    dedupe_key       TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL,
    attempts         INTEGER NOT NULL DEFAULT 0,
    max_attempts     INTEGER NOT NULL,
    next_attempt_at  TEXT NOT NULL,
    last_status_code INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

CREATE INDEX idx_function_invocations_due ON function_invocations (status, next_attempt_at);
CREATE INDEX idx_function_invocations_fn ON function_invocations (function, status);
CREATE UNIQUE INDEX idx_function_invocations_dedupe ON function_invocations (function, dedupe_key) WHERE dedupe_key != '';
