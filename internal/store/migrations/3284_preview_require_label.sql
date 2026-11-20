-- Label a pull request needs before it gets a preview; empty builds every one.
ALTER TABLE preview_app_settings ADD COLUMN require_label TEXT NOT NULL DEFAULT '';
