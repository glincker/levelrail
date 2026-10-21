-- Per-app publish lock (desired state) and the builds it holds ready-not-live; publishing a held build writes its saved desired service.
CREATE TABLE app_publish_locks (
    service_name TEXT PRIMARY KEY,
    reason       TEXT NOT NULL DEFAULT '',
    locked_by    TEXT NOT NULL DEFAULT '',
    locked_at    TEXT NOT NULL,
    pinned_image TEXT NOT NULL DEFAULT ''
);

CREATE TABLE held_builds (
    deploy_id    TEXT PRIMARY KEY,
    service_name TEXT NOT NULL,
    desired_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    published_at TEXT,
    published_by TEXT NOT NULL DEFAULT ''
);
CREATE INDEX held_builds_service ON held_builds (service_name, created_at);
