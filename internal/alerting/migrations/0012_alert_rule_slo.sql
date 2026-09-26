-- kind='slo_burn' rules keep their request-based SLO (objective, target,
-- latency threshold) as JSON; every other kind leaves it empty.
ALTER TABLE alert_rules ADD COLUMN slo_json TEXT NOT NULL DEFAULT '';
