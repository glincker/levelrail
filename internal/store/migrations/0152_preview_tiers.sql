-- Preview tiers: a per-app mode (off, metadata, screenshot) and, per record,
-- where its pixels came from plus the page metadata a card is composed from.
-- Existing settings rows keep their meaning: enabled becomes screenshot.
ALTER TABLE deploy_preview_settings ADD COLUMN mode TEXT NOT NULL DEFAULT '';
UPDATE deploy_preview_settings SET mode = CASE WHEN enabled = 1 THEN 'screenshot' ELSE 'off' END;

ALTER TABLE deploy_previews ADD COLUMN source TEXT NOT NULL DEFAULT 'screenshot';
ALTER TABLE deploy_previews ADD COLUMN meta TEXT NOT NULL DEFAULT '';
