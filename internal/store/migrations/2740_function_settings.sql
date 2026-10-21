-- Per-function request timeout and max concurrency for held wake requests; 0 means use the server default.
CREATE TABLE function_settings (
    service_name            TEXT PRIMARY KEY,
    request_timeout_seconds INTEGER NOT NULL DEFAULT 0,
    max_concurrency         INTEGER NOT NULL DEFAULT 0,
    updated_at              TEXT NOT NULL
);
