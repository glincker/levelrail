-- Sibling service names (within the same app) this service waits on
-- before the reconciler creates its own containers, the same JSON-array
-- shape command/entrypoint already use on this table (0088/0098).
ALTER TABLE desired_services ADD COLUMN depends_on TEXT NOT NULL DEFAULT '[]';
