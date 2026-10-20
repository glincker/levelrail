-- Operator override for this server's public addresses, used for DNS
-- record suggestions and DNS checks when detection is wrong (NAT, a load
-- balancer in front, or a provider that blocks the probe services).
CREATE TABLE server_address_override (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    ipv4       TEXT NOT NULL DEFAULT '',
    ipv6       TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
