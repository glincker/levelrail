-- What the newest container of a service was created with: salted short
-- hashes of the app-owned env values plus scalar config, so the API can tell
-- when desired state has drifted from the running container.
CREATE TABLE IF NOT EXISTS applied_config (
    service_name  TEXT PRIMARY KEY,
    release       TEXT NOT NULL,
    env_hashes    TEXT NOT NULL DEFAULT '{}',
    fields        TEXT NOT NULL DEFAULT '{}',
    applied_at    TEXT NOT NULL
);
