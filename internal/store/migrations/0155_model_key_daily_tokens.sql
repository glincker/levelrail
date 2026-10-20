-- Per-key daily token budget (0 = unlimited) and the principal that created the key.
ALTER TABLE model_keys ADD COLUMN tpd INTEGER NOT NULL DEFAULT 0;
ALTER TABLE model_keys ADD COLUMN created_by TEXT NOT NULL DEFAULT '';
