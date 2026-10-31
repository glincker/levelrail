-- Composite indexes for the busiest list reads. The deployments list and
-- summary probe "latest succeeded attempt of this app" per row, and the
-- audit log filters by path or actor type while ordering by time.
CREATE INDEX IF NOT EXISTS idx_deploy_attempts_service_status_started ON deploy_attempts (service_name, status, started_at DESC, image, source, reason);
CREATE INDEX IF NOT EXISTS idx_deploy_attempts_status_started ON deploy_attempts (status, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_path_created ON audit_log (path, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor_type_created ON audit_log (actor_type, created_at DESC);
