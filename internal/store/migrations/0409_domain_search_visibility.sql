-- A row means the domain asks search engines to stay away: the ingress adds
-- X-Robots-Tag and answers /robots.txt itself. No row is the default (normal).
CREATE TABLE domain_search_visibility (
    domain TEXT PRIMARY KEY REFERENCES service_domains(domain) ON DELETE CASCADE
);
