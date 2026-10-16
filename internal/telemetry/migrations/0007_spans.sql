-- Trace spans received over OTLP, one row per span. resource_id is the app
-- the ingest key belongs to ("service:<name>"), never what the sender claims.
CREATE TABLE spans (
    trace_id       TEXT NOT NULL,
    span_id        TEXT NOT NULL,
    parent_id      TEXT NOT NULL DEFAULT '',
    resource_id    TEXT NOT NULL,
    name           TEXT NOT NULL,
    kind           INTEGER NOT NULL DEFAULT 0,
    start_ns       INTEGER NOT NULL,
    end_ns         INTEGER NOT NULL,
    status_code    INTEGER NOT NULL DEFAULT 0,
    status_message TEXT NOT NULL DEFAULT '',
    attrs          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (trace_id, span_id)
) WITHOUT ROWID;

CREATE INDEX idx_spans_resource_start ON spans (resource_id, start_ns);
CREATE INDEX idx_spans_start ON spans (start_ns);
