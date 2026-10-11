-- Automatic control plane upgrade policy: a singleton row (id = 1). The
-- default is off, so nothing upgrades itself until an operator opts in.
CREATE TABLE upgrade_policy (
    id                      INTEGER PRIMARY KEY CHECK (id = 1),
    mode                    TEXT NOT NULL DEFAULT 'off' CHECK (mode IN ('off', 'patch', 'any')),
    window_cron             TEXT NOT NULL DEFAULT '',
    window_duration_seconds INTEGER NOT NULL DEFAULT 7200,
    window_timezone         TEXT NOT NULL DEFAULT 'UTC',
    min_release_age_days    INTEGER NOT NULL DEFAULT 3,
    notify_channel_ids      TEXT NOT NULL DEFAULT '[]',
    paused                  INTEGER NOT NULL DEFAULT 0,
    updated_by              TEXT NOT NULL DEFAULT '',
    updated_at              TEXT NOT NULL DEFAULT '',
    decision_tag            TEXT NOT NULL DEFAULT '',
    decision                TEXT NOT NULL DEFAULT '',
    decision_at             TEXT NOT NULL DEFAULT ''
);

INSERT INTO upgrade_policy (id) VALUES (1);
