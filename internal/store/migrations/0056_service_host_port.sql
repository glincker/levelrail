-- Optional pinned host port for a service's published container port
-- (internal/docker.PortBinding.HostPort), the operator-facing counterpart
-- to app.yaml's existing port: (container port only). NULL keeps today's
-- behavior: Docker auto-assigns an ephemeral host port on every recreate.
ALTER TABLE desired_services ADD COLUMN host_port INTEGER;
