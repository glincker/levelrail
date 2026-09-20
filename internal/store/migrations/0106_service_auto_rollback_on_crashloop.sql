-- Opt-in per-app auto-rollback: when a crashloop alert rule fires for a
-- freshly-deployed image, internal/alerting can point desired.Image back
-- at the previous known-good tag automatically instead of only alerting.
-- Off by default, like every other opt-in feature toggle in this
-- codebase (preview environments, PR status comments).
ALTER TABLE desired_services ADD COLUMN auto_rollback_on_crashloop INTEGER NOT NULL DEFAULT 0;
