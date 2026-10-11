-- kind='log_pattern' and kind='log_level_rate' rules keep their match config
-- (query, regex, field filters, thresholds, window, cooldown) as JSON; every
-- other kind leaves it empty.
ALTER TABLE alert_rules ADD COLUMN log_rule_json TEXT NOT NULL DEFAULT '';
