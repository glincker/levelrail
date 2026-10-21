-- Function input capture: per-function opt-in to keep the input of test and CLI invocations for replay.
CREATE TABLE function_capture (
    service_name TEXT PRIMARY KEY,
    enabled      INTEGER NOT NULL DEFAULT 0,
    updated_at   TEXT NOT NULL
);
