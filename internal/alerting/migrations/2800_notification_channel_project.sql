-- Optional owner project: a project channel only serves rules and deploy targets of that project's apps.
ALTER TABLE notification_channels ADD COLUMN project_id TEXT NOT NULL DEFAULT '';
