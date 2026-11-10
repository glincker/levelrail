-- Repo-relative compose file for a compose app deployed from git; empty for a single-service source.
ALTER TABLE service_git_sources ADD COLUMN compose_path TEXT NOT NULL DEFAULT '';
