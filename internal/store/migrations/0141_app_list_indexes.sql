-- Supports the filtered app list: environment filter by name and tag
-- filter by tag_id then app_name without scanning app_tags.
CREATE INDEX IF NOT EXISTS idx_environments_name ON environments (name);
CREATE INDEX IF NOT EXISTS idx_app_tags_tag_app ON app_tags (tag_id, app_name);
