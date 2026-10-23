-- Per-app ingress abuse defense: one JSON policy per app (mode, per-client
-- rate, deny lists) and the last status condition the ingress reconciler
-- reported for it.
CREATE TABLE app_ingress_abuse (
    app_name          TEXT PRIMARY KEY REFERENCES desired_services(name) ON DELETE CASCADE,
    spec              TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    updated_by        TEXT NOT NULL DEFAULT '',
    condition_status  TEXT NOT NULL DEFAULT '',
    condition_reason  TEXT NOT NULL DEFAULT '',
    condition_message TEXT NOT NULL DEFAULT '',
    condition_at      TEXT NOT NULL DEFAULT ''
);
