-- Status page v2: component groups, per-component descriptions and branding.
CREATE TABLE status_component_groups (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

ALTER TABLE status_components ADD COLUMN group_id TEXT NOT NULL DEFAULT '';
ALTER TABLE status_components ADD COLUMN description TEXT NOT NULL DEFAULT '';

ALTER TABLE status_page_settings ADD COLUMN accent TEXT NOT NULL DEFAULT '';
ALTER TABLE status_page_settings ADD COLUMN logo_url TEXT NOT NULL DEFAULT '';
ALTER TABLE status_page_settings ADD COLUMN about TEXT NOT NULL DEFAULT '';
ALTER TABLE status_page_settings ADD COLUMN website TEXT NOT NULL DEFAULT '';
ALTER TABLE status_page_settings ADD COLUMN noindex INTEGER NOT NULL DEFAULT 0;
ALTER TABLE status_page_settings ADD COLUMN show_powered_by INTEGER NOT NULL DEFAULT 1;
