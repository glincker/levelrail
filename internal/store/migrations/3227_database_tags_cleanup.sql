-- Database tags live in resource_tags without a foreign key, so drop them with the
-- database; a later database reusing the name must not inherit old tags.
CREATE TRIGGER IF NOT EXISTS trg_desired_databases_drop_tags
AFTER DELETE ON desired_databases
BEGIN
    DELETE FROM resource_tags WHERE kind = 'database' AND name = OLD.name;
END;

DELETE FROM resource_tags
WHERE kind = 'database' AND name NOT IN (SELECT name FROM desired_databases);
