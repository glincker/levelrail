-- Maintenance page copy, Retry-After, bypass rules, and an optional link to a
-- scheduled maintenance window whose end also ends the domain's maintenance.
ALTER TABLE domain_maintenance ADD COLUMN title TEXT NOT NULL DEFAULT '';
ALTER TABLE domain_maintenance ADD COLUMN message TEXT NOT NULL DEFAULT '';
ALTER TABLE domain_maintenance ADD COLUMN retry_after_seconds INTEGER NOT NULL DEFAULT 0;
ALTER TABLE domain_maintenance ADD COLUMN bypass_cidrs TEXT NOT NULL DEFAULT '';
ALTER TABLE domain_maintenance ADD COLUMN bypass_health_check INTEGER NOT NULL DEFAULT 1;
ALTER TABLE domain_maintenance ADD COLUMN window_id TEXT NOT NULL DEFAULT '';
