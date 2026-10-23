-- Per-destination switch for age-sealing database dumps; existing rows stay off until an operator opts in.
ALTER TABLE backup_targets ADD COLUMN encrypt_dumps INTEGER NOT NULL DEFAULT 0;
