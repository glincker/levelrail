-- Ping history and hashed ping token for heartbeat monitors. The plaintext token is never stored.
CREATE TABLE monitor_heartbeats (
    monitor_id    TEXT    PRIMARY KEY,
    token_hash    TEXT    NOT NULL UNIQUE,
    armed_at      INTEGER NOT NULL,
    last_ok_at    INTEGER NOT NULL DEFAULT 0,
    last_start_at INTEGER NOT NULL DEFAULT 0,
    last_fail_at  INTEGER NOT NULL DEFAULT 0
);
