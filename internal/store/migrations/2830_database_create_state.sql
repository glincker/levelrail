-- Settings chosen at create time are stored with the record. Rows that
-- existed before this migration already have their volume, so they default
-- to resolved.
ALTER TABLE desired_databases ADD COLUMN existing_volume TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_databases ADD COLUMN volume_resolved INTEGER NOT NULL DEFAULT 1;
