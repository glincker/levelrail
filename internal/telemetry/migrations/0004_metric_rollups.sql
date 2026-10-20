-- Time-bucketed rollups of metric_samples. tier is the bucket width in
-- seconds (60 or 3600); ts is the bucket start. Sum-kind metrics (see
-- IsSumMetric) read back as sum, everything else as sum/count.
CREATE TABLE metric_rollups (
    tier        INTEGER NOT NULL,
    resource_id TEXT NOT NULL,
    metric      TEXT NOT NULL,
    ts          INTEGER NOT NULL,
    sum         REAL NOT NULL,
    min         REAL NOT NULL,
    max         REAL NOT NULL,
    count       INTEGER NOT NULL,
    PRIMARY KEY (tier, resource_id, metric, ts)
) WITHOUT ROWID;

CREATE INDEX idx_metric_rollups_ts ON metric_rollups (tier, ts);

-- done_through is the exclusive end (unix seconds) of the last fully rolled
-- up bucket for a tier: the resume point after an interrupted run.
CREATE TABLE rollup_state (
    tier         INTEGER PRIMARY KEY,
    done_through INTEGER NOT NULL
);
