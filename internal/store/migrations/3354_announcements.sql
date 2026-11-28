-- Verified announcement feed cache plus per-user read and snooze state.
-- Dismissals reuse attention_dismissals under the key announcement:<id>.
CREATE TABLE announcement_feed_state (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    last_sequence INTEGER NOT NULL DEFAULT 0,
    etag          TEXT NOT NULL DEFAULT '',
    fetched_at    TEXT NOT NULL DEFAULT '',
    verified_at   TEXT NOT NULL DEFAULT '',
    expires_at    TEXT NOT NULL DEFAULT '',
    last_error    TEXT NOT NULL DEFAULT ''
);

INSERT INTO announcement_feed_state (id) VALUES (1);

CREATE TABLE announcements (
    id            TEXT PRIMARY KEY,
    tier          TEXT NOT NULL,
    kind          TEXT NOT NULL,
    payload_json  TEXT NOT NULL,
    issued_at     TEXT NOT NULL,
    expires_at    TEXT NOT NULL,
    seen_sequence INTEGER NOT NULL
);

CREATE TABLE announcement_revoked_keys (
    key_id     TEXT PRIMARY KEY,
    revoked_at TEXT NOT NULL,
    sequence   INTEGER NOT NULL
);

CREATE TABLE announcement_user_state (
    user_id         TEXT NOT NULL,
    announcement_id TEXT NOT NULL,
    read_at         TEXT NOT NULL DEFAULT '',
    snoozed_until   TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, announcement_id)
);
