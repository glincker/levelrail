-- Binds an app's database attachment to the database instance it was written against.
ALTER TABLE desired_services ADD COLUMN database_attachment_instance TEXT NOT NULL DEFAULT '';
