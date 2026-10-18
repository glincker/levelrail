-- Build center: which node ran a build, for which platform, and how big the image came out.
ALTER TABLE build_runs ADD COLUMN node_id TEXT NOT NULL DEFAULT '';
ALTER TABLE build_runs ADD COLUMN platform TEXT NOT NULL DEFAULT '';
ALTER TABLE build_runs ADD COLUMN image_bytes INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_build_runs_tag ON build_runs (tag);
