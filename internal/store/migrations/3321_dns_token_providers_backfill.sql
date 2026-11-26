-- The token DNS providers added with #1681 (porkbun, desec, godaddy,
-- namecheap) never got rows on installs that predate this migration:
-- 3290 seeded only digitalocean, hetzner and azure. GetDNSTokenProviderEnabled
-- surfaced the missing row as sql.ErrNoRows, aborting the whole DNS provider
-- resolution and breaking zone listing in the panel.
INSERT OR IGNORE INTO dns_token_providers (provider) VALUES
    ('porkbun'), ('desec'), ('godaddy'), ('namecheap');
