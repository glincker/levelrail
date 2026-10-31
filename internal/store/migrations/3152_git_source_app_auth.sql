-- Stored git sources default to GitHub App access; a team scoped writer turns it off.
ALTER TABLE service_git_sources ADD COLUMN app_auth INTEGER NOT NULL DEFAULT 1;
