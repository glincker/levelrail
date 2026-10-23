-- Static sites can run a build command and serve its publish directory.
ALTER TABLE service_git_sources ADD COLUMN build_command TEXT NOT NULL DEFAULT '';
ALTER TABLE service_git_sources ADD COLUMN publish_dir TEXT NOT NULL DEFAULT '';
