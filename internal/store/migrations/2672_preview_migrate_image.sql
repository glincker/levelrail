-- Per-app preview migration step and prebuilt image tag. migrate_command runs
-- once after the preview database is ready; image_tag_template deploys a
-- registry tag for the pull request instead of building. Both default off.
ALTER TABLE preview_app_settings ADD COLUMN migrate_command TEXT NOT NULL DEFAULT '';
ALTER TABLE preview_app_settings ADD COLUMN migrate_timeout_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE preview_app_settings ADD COLUMN image_tag_template TEXT NOT NULL DEFAULT '';
