-- An ongoing window backs open ended domain maintenance on the status page; it has no real end until maintenance does.
ALTER TABLE maintenance_windows ADD COLUMN ongoing INTEGER NOT NULL DEFAULT 0;
