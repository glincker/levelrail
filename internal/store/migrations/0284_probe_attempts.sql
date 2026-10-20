-- One row per individual readiness-probe attempt during a deploy's
-- cutover (internal/probe.WithOnAttempt, wired from
-- internal/reconcile/application's waitReady), the per-attempt detail
-- (status code, latency) a reconcile condition's own single
-- Reason/Message summary never captures. Purely an additional
-- observability side channel: WaitReady's own retry/timeout/pass-fail
-- logic is unchanged by this table's existence.
--
-- deploy_attempt_id is resolved the same way RecordRollout/
-- RecordRolloutFailure already resolve their own target row
-- (migrations/0138_deploy_safety.sql): the newest succeeded attempt for
-- (service_name, image) at record time, since waitReady has no
-- deploy_attempt_id of its own to thread through Reconcile. REFERENCES
-- deploy_attempts(id), the same same-database FK convention
-- backup_history already establishes for target_id
-- (migrations/0018_backup_targets.sql): both tables live in
-- levelrail.db, unlike telemetry.db's deploy_logs, which can't use a
-- real FK for its own cross-database attempt_id reference.
CREATE TABLE probe_attempts (
    id                INTEGER PRIMARY KEY,
    deploy_attempt_id TEXT NOT NULL REFERENCES deploy_attempts(id),
    target            TEXT NOT NULL,
    success           INTEGER NOT NULL,
    status_code       INTEGER NOT NULL DEFAULT 0,
    exit_code         INTEGER NOT NULL DEFAULT 0,
    error             TEXT NOT NULL DEFAULT '',
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    probed_at         TEXT NOT NULL
);

-- Supports ListProbeAttempts' only read shape: every attempt for one
-- deploy, oldest first.
CREATE INDEX idx_probe_attempts_deploy_attempt_probed ON probe_attempts (deploy_attempt_id, probed_at);
