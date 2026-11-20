-- Service keys a multi-service preview leaves out, comma separated.
ALTER TABLE preview_app_settings ADD COLUMN exclude_services TEXT NOT NULL DEFAULT '';
