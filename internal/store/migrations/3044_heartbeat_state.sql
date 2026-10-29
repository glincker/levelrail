-- Dead-man heartbeat: last ping outcome and the body toggle. The ping URL itself
-- lives in the secrets store because monitor URLs carry the secret key.
CREATE TABLE heartbeat_state (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    body_enabled INTEGER NOT NULL DEFAULT 1,
    last_ping_at TEXT NOT NULL DEFAULT '',
    last_result  TEXT NOT NULL DEFAULT '',
    last_error   TEXT NOT NULL DEFAULT ''
);
INSERT INTO heartbeat_state (id) VALUES (1);
