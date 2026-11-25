-- Automatic upgrades wait one day after a release by default, not three.
-- Only rows still on the old default move; a chosen value is kept.
UPDATE upgrade_policy SET min_release_age_days = 1 WHERE min_release_age_days = 3;
