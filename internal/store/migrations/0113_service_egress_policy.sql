-- Outbound network allowlist (internal/spec.Egress), stored as JSON the
-- same way desired_services.health/hooks already are (0002, 0082): a nil
-- store.DesiredService.Egress marshals to the JSON literal null, so an
-- existing row picks that up for free as "unrestricted egress" after
-- this migration runs, the opt-in default every app already has.
ALTER TABLE desired_services ADD COLUMN egress_policy TEXT NOT NULL DEFAULT 'null';
