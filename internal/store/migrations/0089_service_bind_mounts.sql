-- Bind-mount support for ordinary application services (internal/compose's
-- volumes: short form, an absolute host path on the left side), distinct
-- from the existing named-volume-only volumes column (migrations/0041):
-- store.ServiceBindMount's own doc comment covers why this is a separate
-- type rather than an optional field there. Same JSON-array storage shape
-- volumes already uses, and same "declarative, resolved before storing"
-- treatment.
ALTER TABLE desired_services ADD COLUMN bind_mounts TEXT NOT NULL DEFAULT '[]';
