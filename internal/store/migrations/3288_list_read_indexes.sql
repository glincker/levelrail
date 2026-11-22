-- Composite indexes for list queries that scanned or sorted in a temp b-tree.
CREATE INDEX IF NOT EXISTS idx_deploy_attempts_service_started_id ON deploy_attempts (service_name, started_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor_name_created ON audit_log (actor_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor_id_created ON audit_log (actor_type, actor_id, created_at DESC);
