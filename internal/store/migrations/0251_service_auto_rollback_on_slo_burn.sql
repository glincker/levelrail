-- Opt-in per-app auto-rollback on an SLO burn-rate alert, mirroring
-- 0106_service_auto_rollback_on_crashloop.sql's own reasoning but with a
-- mode instead of a plain bool: 'off' (default), 'auto' (roll back
-- immediately, same as crashloop), 'dry_run' (log what would have
-- happened, never deploy), or 'pause_for_human' (open a pending
-- store.DeployApproval instead of deploying directly).
ALTER TABLE desired_services ADD COLUMN auto_rollback_on_slo_burn TEXT NOT NULL DEFAULT 'off';
