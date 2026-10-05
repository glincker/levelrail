-- Every hostname the ingress controller last applied to Caddy, so the orphan
-- reaper can tell a certificate nothing serves any more from a live one.
CREATE TABLE served_hosts_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    hosts      TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
