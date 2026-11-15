-- Extra internal DNS names an app answers to, a JSON list; null means none.
ALTER TABLE desired_services ADD COLUMN internal_dns_names TEXT NOT NULL DEFAULT 'null';
