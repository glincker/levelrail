-- Per-app preview policy for the PR preview lifecycle: its own cap, smaller
-- default resources, idle sleep, database strategy, secrets for forks, the
-- optional basic auth gate and search visibility. Defaults are the safe
-- choice: no production data, no fork secrets, hidden from search.
ALTER TABLE preview_app_settings ADD COLUMN max_previews INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_app_settings ADD COLUMN memory_limit TEXT NOT NULL DEFAULT '';
ALTER TABLE preview_app_settings ADD COLUMN cpu_limit REAL NOT NULL DEFAULT 0;
ALTER TABLE preview_app_settings ADD COLUMN idle_sleep_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_app_settings ADD COLUMN database_strategy TEXT NOT NULL DEFAULT 'none';
ALTER TABLE preview_app_settings ADD COLUMN seed_database TEXT NOT NULL DEFAULT '';
ALTER TABLE preview_app_settings ADD COLUMN allow_fork_secrets INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_app_settings ADD COLUMN gate_basic_auth INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_app_settings ADD COLUMN gate_username TEXT NOT NULL DEFAULT '';
ALTER TABLE preview_app_settings ADD COLUMN allow_indexing INTEGER NOT NULL DEFAULT 0;
