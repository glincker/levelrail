-- Platform-wide Route53 DNS-01 config, the same single-seeded-row shape
-- as cloudflare_dns_settings (migrations/0051): a second, independent
-- ACME DNS-01 provider, not a replacement. Region and hosted_zone_id are
-- both optional (libdns/route53 resolves either on its own when empty)
-- and are not secret, so they live here rather than in internal/secrets.
-- The access key ID and secret access key go through internal/secrets,
-- keyed by store.Route53DNSSecretsKey().
CREATE TABLE route53_dns_settings (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    enabled        INTEGER NOT NULL DEFAULT 0,
    region         TEXT NOT NULL DEFAULT '',
    hosted_zone_id TEXT NOT NULL DEFAULT ''
);

INSERT INTO route53_dns_settings (id) VALUES (1);
