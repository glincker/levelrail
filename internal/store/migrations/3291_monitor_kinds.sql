-- The first monitors table only allowed http and tcp. Rebuild it without the kind CHECK so
-- heartbeat, tls, dns and ping monitors can be stored; the service validates the kind.
CREATE TABLE monitors_new (
    id          TEXT    PRIMARY KEY,
    name        TEXT    NOT NULL,
    kind        TEXT    NOT NULL,
    interval_s  INTEGER NOT NULL,
    timeout_ms  INTEGER NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 1,
    spec        TEXT    NOT NULL,
    created_at  INTEGER NOT NULL
);
INSERT INTO monitors_new SELECT id, name, kind, interval_s, timeout_ms, enabled, spec, created_at FROM monitors;
DROP TABLE monitors; -- compat-break: rebuilt with every row to widen the kind check
ALTER TABLE monitors_new RENAME TO monitors;
