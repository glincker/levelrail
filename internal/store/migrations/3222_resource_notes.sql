-- Operator description and markdown notes for apps and databases, keyed by kind and name like resource_labels.
CREATE TABLE resource_notes (
	kind        TEXT NOT NULL,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	notes       TEXT NOT NULL DEFAULT '',
	updated_by  TEXT NOT NULL DEFAULT '',
	updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
	PRIMARY KEY (kind, name)
);

-- Triggers catch every delete path, so a new resource reusing the name never shows old notes.
CREATE TRIGGER desired_services_drop_notes AFTER DELETE ON desired_services
BEGIN
	DELETE FROM resource_notes WHERE kind = 'app' AND name = OLD.name;
END;

CREATE TRIGGER desired_databases_drop_notes AFTER DELETE ON desired_databases
BEGIN
	DELETE FROM resource_notes WHERE kind = 'database' AND name = OLD.name;
END;
