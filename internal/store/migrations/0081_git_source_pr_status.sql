-- Opt-in GitHub PR comment/commit status notifications for preview
-- environments (docs/roadmap.md's Preview environments entry). Lives on
-- service_git_sources alongside preview_enabled (migrations/0064), same
-- reasoning: only ever meaningful for an app that already has a
-- connected source, off by default.
ALTER TABLE service_git_sources ADD COLUMN post_pr_comments INTEGER NOT NULL DEFAULT 0;
