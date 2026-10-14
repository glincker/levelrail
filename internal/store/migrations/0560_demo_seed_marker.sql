-- Single-row marker written only by the demo seeder; demo mode refuses to
-- start without it, so a real instance can never be switched into demo mode.
CREATE TABLE demo_seed (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    seed      INTEGER NOT NULL,
    anchor    TEXT    NOT NULL,
    seeded_at TEXT    NOT NULL
);
