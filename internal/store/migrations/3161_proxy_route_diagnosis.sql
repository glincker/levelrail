-- Structured certificate diagnosis stored with each managed proxy route probe.
ALTER TABLE proxy_route_status ADD COLUMN cert_class TEXT NOT NULL DEFAULT '';
ALTER TABLE proxy_route_status ADD COLUMN diagnosis TEXT NOT NULL DEFAULT '';
ALTER TABLE proxy_route_status ADD COLUMN hint TEXT NOT NULL DEFAULT '';
ALTER TABLE proxy_route_status ADD COLUMN snippet TEXT NOT NULL DEFAULT '';
ALTER TABLE proxy_route_status ADD COLUMN evidence TEXT NOT NULL DEFAULT '[]';
