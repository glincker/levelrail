-- One row per (resource, metric, timestamp) sample. WITHOUT ROWID: the
-- composite primary key already uniquely orders every row the way
-- range queries actually read them (all samples for one resource+metric
-- across a time window, ADR 009), so a separate rowid index would just
-- be overhead SQLite doesn't need to pay here.
--
-- resource_id is a plain string, not a foreign key into desired_services
-- (internal/store, a different database file per ADR 009): telemetry.db
-- must keep collecting and stay queryable even for a resource whose
-- desired state has since been deleted, since "what happened right
-- before this app was removed" is exactly the kind of question this
-- store exists to answer.
CREATE TABLE metric_samples (
    resource_id TEXT NOT NULL,
    metric      TEXT NOT NULL,
    ts          INTEGER NOT NULL,
    value       REAL NOT NULL,
    PRIMARY KEY (resource_id, metric, ts)
) WITHOUT ROWID;

-- Supports the retention sweep's DELETE WHERE ts < ? without a full
-- table scan; the primary key above already orders by resource_id and
-- metric first, so a pure-timestamp sweep across every resource needs
-- its own index.
CREATE INDEX idx_metric_samples_ts ON metric_samples (ts);
