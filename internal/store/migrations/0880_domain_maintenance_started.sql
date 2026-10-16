-- When maintenance began and when it switches itself off (RFC 3339, empty when unknown or open ended).
ALTER TABLE domain_maintenance ADD COLUMN started_at TEXT NOT NULL DEFAULT '';
ALTER TABLE domain_maintenance ADD COLUMN auto_off_at TEXT NOT NULL DEFAULT '';
