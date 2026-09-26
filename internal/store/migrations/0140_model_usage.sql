-- Hourly gateway usage per model and key. hour_start is the unix time of
-- the hour. Token columns only count requests whose response carried a
-- usage object (usage_requests); requests without one add to requests only.
CREATE TABLE model_usage_hourly (
    model_name        TEXT NOT NULL,
    key_id            TEXT NOT NULL,
    hour_start        INTEGER NOT NULL,
    requests          INTEGER NOT NULL DEFAULT 0,
    status_2xx        INTEGER NOT NULL DEFAULT 0,
    status_4xx        INTEGER NOT NULL DEFAULT 0,
    status_5xx        INTEGER NOT NULL DEFAULT 0,
    rate_limited      INTEGER NOT NULL DEFAULT 0,
    usage_requests    INTEGER NOT NULL DEFAULT 0,
    input_tokens      INTEGER NOT NULL DEFAULT 0,
    output_tokens     INTEGER NOT NULL DEFAULT 0,
    bytes_out         INTEGER NOT NULL DEFAULT 0,
    duration_ms_sum   INTEGER NOT NULL DEFAULT 0,
    ttft_ms_sum       INTEGER NOT NULL DEFAULT 0,
    ttft_count        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (model_name, key_id, hour_start)
);

CREATE INDEX idx_model_usage_hour ON model_usage_hourly(hour_start);
