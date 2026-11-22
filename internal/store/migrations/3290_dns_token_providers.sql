-- Enabled flag per DNS provider connected with one stored credential
-- (DigitalOcean, Hetzner, Azure). The credential itself goes through
-- internal/secrets under store.DNSTokenSecretsKey(provider).
CREATE TABLE dns_token_providers (
    provider TEXT PRIMARY KEY,
    enabled  INTEGER NOT NULL DEFAULT 0
);

INSERT INTO dns_token_providers (provider) VALUES ('digitalocean'), ('hetzner'), ('azure');
