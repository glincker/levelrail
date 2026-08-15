-- One row per deploy-attempt build/log output line. A deliberate sibling
-- of log_entries (0002), not a reuse of it: see
-- docs-local/research/deploy-attempt-id-and-log-persistence.md section 2
-- for why. log_entries is an open-ended, restart-scoped stream for a
-- running container, with its own 15-day rolling retention default and
-- an FTS5 index for interactive search across a live stream. A deploy
-- attempt's log is the opposite shape: one bounded, terminal event
-- (attempt starts, produces some output, finishes once, forever) that
-- exists to be replayed in full from the start, not searched or
-- tailed indefinitely, and needs a retention policy tuned to "keep my
-- last N deploy attempts", not "keep the last 15 days of a live
-- stream." Sharing one table would mean every query disambiguating
-- build rows from container rows, and one retention sweep serving two
-- genuinely different lifetimes.
--
-- attempt_id is a plain string, not a foreign key to
-- levelrail.db's deploy_attempts table: it can't be one at all
-- (SQLite has no cross-database foreign keys, and deploy_attempts lives
-- in a different database file, levelrail.db, per the same
-- control-plane-history-vs-telemetry split ADR 008/009 already draw),
-- which is also exactly the same non-FK convention every other
-- resource_id-shaped column in this package already follows for its own,
-- different reason (this store must keep serving a resource whose
-- desired state has since been deleted).
--
-- No FTS index (contrast log_entries_fts): full-text search across one
-- bounded build's output has no real use case yet (the frontend contract
-- this implements, web/src/hooks/useDeployLogStream.ts, is full replay
-- plus live tail, not search), so this stays the simpler of the two log
-- shapes until a real need for search appears.
CREATE TABLE deploy_logs (
    id         INTEGER PRIMARY KEY,
    attempt_id TEXT NOT NULL,
    stream     TEXT NOT NULL,
    ts         INTEGER NOT NULL, -- unix nanoseconds, same sub-second-ordering reasoning as log_entries.ts
    message    TEXT NOT NULL
);

-- Supports both QueryDeployLog (attempt_id, ordered by ts) and
-- RetainDeployLogs (a pure ts sweep still benefits from attempt_id being
-- first in a composite index for the common query shape, the same
-- reasoning idx_log_entries_resource_ts's own comment gives).
CREATE INDEX idx_deploy_logs_attempt_ts ON deploy_logs (attempt_id, ts);
