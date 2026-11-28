-- Which app instance owns a name, so history kept after a delete is not inherited by a new app that reuses the name.
CREATE TABLE service_instances (
    name        TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL
);

INSERT INTO service_instances (name, instance_id) SELECT name, lower(hex(randomblob(8))) FROM desired_services;

-- Volume backups of apps that still exist belong to them; rows of apps deleted earlier stay unowned.
UPDATE backup_history
SET instance_id = (SELECT instance_id FROM service_instances s WHERE s.name = backup_history.service_name)
WHERE resource_kind = 'volume' AND instance_id = '' AND service_name IN (SELECT name FROM service_instances);
