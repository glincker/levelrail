-- Pre-v2 status pages were always noindex, so keep existing rows that way after
-- the upgrade. Operators switch indexing on explicitly from the status page settings.
UPDATE status_page_settings SET noindex = 1;
