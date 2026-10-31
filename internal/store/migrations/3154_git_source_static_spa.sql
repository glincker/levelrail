-- A static git source can fall back to index.html for unknown paths (single page app mode).
ALTER TABLE service_git_sources ADD COLUMN static_spa INTEGER NOT NULL DEFAULT 0;
