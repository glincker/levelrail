-- Latest migration parity report (names, counts and probe results only, never env values). One row, replaced on each run.
CREATE TABLE migration_verify_report (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    platform     TEXT NOT NULL,
    generated_at TEXT NOT NULL,
    verdict      TEXT NOT NULL,
    report       TEXT NOT NULL
);
