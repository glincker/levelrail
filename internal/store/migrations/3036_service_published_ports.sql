-- Extra host-published ports per service as a JSON array of
-- {container_port, host_port, protocol}. Existing rows keep their single
-- routed tcp port and start with none.
ALTER TABLE desired_services ADD COLUMN published_ports TEXT NOT NULL DEFAULT '[]';
