-- What the newest container of a service was created with: salted short
-- hashes of the app-owned env values plus scalar config, so the API can tell
-- when desired state has drifted from the running container. held_until is
-- when the held previous release is due for removal, empty when none is held.
CREATE TABLE IF NOT EXISTS applied_config (
    service_name  TEXT PRIMARY KEY,
    release       TEXT NOT NULL,
    env_hashes    TEXT NOT NULL DEFAULT '{}',
    fields        TEXT NOT NULL DEFAULT '{}',
    applied_at    TEXT NOT NULL,
    held_until    TEXT NOT NULL DEFAULT ''
);
