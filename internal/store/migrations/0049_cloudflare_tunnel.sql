-- Platform-wide Cloudflare Tunnel config: whether the operator wants
-- Levelrail's cloudflared container running, mirroring email_settings'
-- (migrations/0032) single seeded row. No token column: the tunnel
-- token goes through internal/secrets instead, keyed by
-- store.CloudflareTunnelSecretsKey(). No status column either: unlike
-- Enabled (an operator's own choice), connection status is derived
-- state, and this codebase already has a place for derived reconcile
-- state (reconcile_status, migrations/0001) that GET
-- /api/v1/databases/{name}/status already reads the same way instead of
-- duplicating it into a settings column.
CREATE TABLE cloudflare_tunnel_settings (
    id      INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0
);

INSERT INTO cloudflare_tunnel_settings (id) VALUES (1);
