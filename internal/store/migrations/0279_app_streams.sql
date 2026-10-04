-- Raw TCP stream forwarding: host_port on this control plane's own host
-- forwards to container_port on the named service's container, proxied
-- by Caddy's layer4 app (internal/ingress) alongside the existing HTTP
-- routes, never a second proxy process. v1 scope: tcp only, protocol is
-- still a column (not an enum constraint) so udp can be added later
-- without a schema change.
--
-- No per-row node_id: a stream always targets the one container Docker
-- already runs the service on (desired_services has no multi-node
-- placement yet either), so there is nothing to disambiguate.
CREATE TABLE app_streams (
    id             TEXT PRIMARY KEY,
    service_name   TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    container_port INTEGER NOT NULL,
    host_port      INTEGER NOT NULL UNIQUE,
    protocol       TEXT NOT NULL DEFAULT 'tcp',
    created_at     TEXT NOT NULL
);

CREATE INDEX idx_app_streams_service_name ON app_streams(service_name);
