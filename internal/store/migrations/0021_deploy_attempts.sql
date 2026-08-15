-- One row per deploy attempt: the plain image-tag trigger
-- (handleTriggerDeploy), the manual git-source build trigger
-- (handleTriggerBuild), and the unattended git-push webhook receiver
-- (internal/webhook) all mint one of these, closing the gap
-- docs-local/research/deploy-attempt-id-and-log-persistence.md documents
-- (see that note's section 1): reconcile_status/UpsertConditions only
-- ever keeps the latest condition per (controller, type) pair, never a
-- row-per-attempt history, so an operator could never see "attempt 3
-- failed, attempt 4 succeeded" for the same app, only "currently Ready".
--
-- This is control-plane history state, the same category as
-- reconcile_status (id is a mint-time identity, not a hash of anything,
-- see store.NewDeployAttemptID), not raw telemetry: it belongs in
-- levelrail.db, not telemetry.db. Its log content (build output) is a
-- separate, purpose-built table in telemetry.db instead
-- (telemetry/migrations/0003_deploy_logs.sql), keyed by this table's id
-- as a plain string, the same non-FK convention every telemetry table
-- already uses for resource_id.
--
-- id is TEXT, not INTEGER AUTOINCREMENT: an opaque, mint-time-random
-- identifier (store.NewDeployAttemptID, matching internal/api/tokens.go's
-- randomTokenID exactly: crypto/rand bytes, base64 URL encoding, a short
-- prefix) rather than a database-assigned sequence, so an attempt's ID is
-- known and stable the moment a trigger handler mints it, before the row
-- is even inserted, letting it be threaded through to a
-- deploylog.Recorder and referenced in a client-visible URL
-- (/deploys/{id}/logs) in the same call.
--
-- finished_at and error are both nullable: a row starts life as
-- status='running' (or 'succeeded' immediately, for the plain image-tag
-- path with no real build step, see internal/api/deploys.go's
-- handleTriggerDeploy doc comment for why that path still gets a row)
-- with both NULL, and is updated exactly once, when the triggering
-- handler's call to internal/deploy.Pipeline.Deploy returns.
CREATE TABLE deploy_attempts (
    id           TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    image        TEXT NOT NULL,
    status       TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    finished_at  TEXT,
    error        TEXT
);

-- Supports ListDeployAttempts' "newest first, for one service" query
-- shape, the only read pattern this table serves today.
CREATE INDEX idx_deploy_attempts_service_started ON deploy_attempts (service_name, started_at DESC);
