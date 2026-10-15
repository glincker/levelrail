-- Whether the last verification proved the proxy hands the domain's requests
-- to the built-in ingress (a marker only the ingress answers came back).
ALTER TABLE proxy_route_status ADD COLUMN delivered INTEGER NOT NULL DEFAULT 0;
