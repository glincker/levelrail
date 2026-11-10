-- Per service container user and working directory; empty keeps the image's USER and WORKDIR.
ALTER TABLE desired_services ADD COLUMN run_user TEXT NOT NULL DEFAULT '';
ALTER TABLE desired_services ADD COLUMN working_dir TEXT NOT NULL DEFAULT '';
