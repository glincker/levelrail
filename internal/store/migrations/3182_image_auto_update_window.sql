-- Optional apply window for image auto-update: a cron start plus a length in
-- minutes. Empty cron keeps the old behavior (apply as soon as a tag moves).
ALTER TABLE image_auto_updates ADD COLUMN window_cron TEXT NOT NULL DEFAULT '';
ALTER TABLE image_auto_updates ADD COLUMN window_minutes INTEGER NOT NULL DEFAULT 0;
