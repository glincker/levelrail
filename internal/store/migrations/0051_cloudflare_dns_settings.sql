-- Platform-wide Cloudflare DNS-01 config, the same single-seeded-row
-- shape as cloudflare_tunnel_settings (migrations/0049). Separate table
-- and separate secret because the credential is a different kind of
-- token: a scoped Cloudflare API token (Zone:DNS:Edit) for the ACME
-- DNS-01 challenge, not the cloudflared connector token
-- cloudflare_tunnel_settings' token authenticates. No token column
-- here either: it goes through internal/secrets, keyed by
-- store.CloudflareDNSSecretsKey().
CREATE TABLE cloudflare_dns_settings (
    id      INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0
);

INSERT INTO cloudflare_dns_settings (id) VALUES (1);
