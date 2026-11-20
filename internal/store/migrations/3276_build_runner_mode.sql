-- Explicit build placement per app: auto, control_plane or node.
ALTER TABLE app_build_runner ADD COLUMN mode TEXT NOT NULL DEFAULT 'auto';
