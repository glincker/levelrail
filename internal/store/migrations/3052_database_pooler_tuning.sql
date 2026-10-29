-- Per-database pooler sizing; 0 means use the control plane env default.
ALTER TABLE desired_databases ADD COLUMN pooler_pool_size INTEGER NOT NULL DEFAULT 0;
ALTER TABLE desired_databases ADD COLUMN pooler_max_client_conn INTEGER NOT NULL DEFAULT 0;
