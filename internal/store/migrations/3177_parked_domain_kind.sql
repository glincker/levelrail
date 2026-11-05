-- A parked domain remembers whether it left an app or a static site, so the
-- list can say where it came from. Rows parked before this are apps.
ALTER TABLE parked_domains ADD COLUMN parked_from_kind TEXT NOT NULL DEFAULT 'app';
