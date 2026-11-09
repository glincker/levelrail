-- Tag and branch trigger patterns; empty keeps the trigger mode's built-in match.
ALTER TABLE service_git_sources ADD COLUMN trigger_tag_pattern TEXT NOT NULL DEFAULT '';
ALTER TABLE service_git_sources ADD COLUMN trigger_branch_pattern TEXT NOT NULL DEFAULT '';
