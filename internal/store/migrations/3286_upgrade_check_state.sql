-- Release lookup health for automatic updates: a singleton row (id = 1), so a
-- restart keeps the failure streak, the backoff and the alert state.
CREATE TABLE upgrade_check_state (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    last_check_at    TEXT NOT NULL DEFAULT '',
    last_ok          INTEGER NOT NULL DEFAULT 0,
    last_error       TEXT NOT NULL DEFAULT '',
    failures         INTEGER NOT NULL DEFAULT 0,
    next_check_at    TEXT NOT NULL DEFAULT '',
    last_good_at     TEXT NOT NULL DEFAULT '',
    last_source      TEXT NOT NULL DEFAULT ''
);

INSERT INTO upgrade_check_state (id) VALUES (1);
