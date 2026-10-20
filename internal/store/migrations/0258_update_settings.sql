-- Platform-wide self-update configuration: which release channel
-- GET /api/v1/updates and internal/updatecheck.Scheduler compare the
-- running build against, and whether the scheduler should even run its
-- periodic check. Same singleton-row shape as ingress_settings
-- (0024_ingress_settings.sql): one authoritative row, id = 1, seeded by
-- this migration so GetUpdateSettings never returns a not-found error.
CREATE TABLE update_settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    channel             TEXT NOT NULL DEFAULT 'stable',
    auto_update_enabled INTEGER NOT NULL DEFAULT 0
);

INSERT INTO update_settings (id, channel, auto_update_enabled) VALUES (1, 'stable', 0);
