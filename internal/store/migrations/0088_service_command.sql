-- Command override for a service's container, the same JSON-array shape
-- domains/secret_env already use on this table (0004/0006): an empty
-- desired_services.command means "run the image's own default CMD",
-- populated from a compose service's command: (internal/compose).
ALTER TABLE desired_services ADD COLUMN command TEXT NOT NULL DEFAULT '[]';
