-- Entrypoint override for a service's container, same JSON-array shape
-- and empty-means-default semantics as the command column (0088),
-- populated from a compose service's entrypoint: (internal/compose).
ALTER TABLE desired_services ADD COLUMN entrypoint TEXT NOT NULL DEFAULT '[]';
