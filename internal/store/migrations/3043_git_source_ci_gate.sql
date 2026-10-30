-- Opt-in wait for CI checks before a git push auto-deploys. ci_required_checks
-- is a JSON array of check names; empty means every check on the commit.
ALTER TABLE service_git_sources ADD COLUMN wait_for_ci INTEGER NOT NULL DEFAULT 0;
ALTER TABLE service_git_sources ADD COLUMN ci_required_checks TEXT NOT NULL DEFAULT '[]';
