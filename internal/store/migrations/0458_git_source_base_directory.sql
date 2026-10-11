-- Build context subdirectory for a connected git source. Empty means the
-- repository root. build_path stays the Dockerfile (or static output
-- directory) path and is always relative to the repository root.
ALTER TABLE service_git_sources ADD COLUMN base_directory TEXT NOT NULL DEFAULT '';
