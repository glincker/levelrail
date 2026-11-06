-- Who started a cleanup run, nullable so runs recorded before this stay valid.
ALTER TABLE janitor_runs ADD COLUMN actor_name TEXT;
ALTER TABLE janitor_runs ADD COLUMN actor_kind TEXT;
