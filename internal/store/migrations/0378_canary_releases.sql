-- One in-flight canary per app: a hidden clone service (canary_service) that
-- receives weight percent of the app's traffic until promoted or aborted.
CREATE TABLE IF NOT EXISTS canary_releases (
    service_name   TEXT PRIMARY KEY,
    canary_service TEXT NOT NULL,
    image          TEXT NOT NULL,
    weight         INTEGER NOT NULL,
    created_at     TEXT NOT NULL
);
