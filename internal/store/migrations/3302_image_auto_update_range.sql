-- Optional semver range for image auto-update (for example "1.x"): the newest
-- release tag inside it is deployed. Empty keeps the old digest-of-the-tag watch.
ALTER TABLE image_auto_updates ADD COLUMN version_range TEXT NOT NULL DEFAULT '';
