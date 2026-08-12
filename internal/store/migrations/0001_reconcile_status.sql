-- CLAUDE.md 4.2: "Every reconcile emits a status condition with a reason
-- string, stored and shown in the UI." This is that storage. A resource
-- can report more than one named condition (Ready, Available, ...), so
-- the primary key is the pair, not controller_name alone.
CREATE TABLE reconcile_status (
    controller_name TEXT NOT NULL,
    condition_type  TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('True', 'False', 'Unknown')),
    reason          TEXT NOT NULL,
    message         TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL,
    PRIMARY KEY (controller_name, condition_type)
);
