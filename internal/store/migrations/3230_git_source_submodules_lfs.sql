-- Opt-in fetch of git submodules and LFS objects for git source builds; off by default.
ALTER TABLE service_git_sources ADD COLUMN fetch_submodules INTEGER NOT NULL DEFAULT 0;
ALTER TABLE service_git_sources ADD COLUMN fetch_lfs INTEGER NOT NULL DEFAULT 0;
