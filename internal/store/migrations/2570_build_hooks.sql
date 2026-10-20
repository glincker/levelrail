-- Named incoming build hooks per app: a secret URL that triggers a git build and deploy. Only the token hash is stored.
CREATE TABLE build_hooks (
    id                TEXT PRIMARY KEY,
    service_name      TEXT NOT NULL,
    name              TEXT NOT NULL,
    branch            TEXT NOT NULL DEFAULT '',
    token_hash        TEXT NOT NULL,
    created_at        TEXT NOT NULL,
    last_triggered_at TEXT,
    last_result       TEXT NOT NULL DEFAULT '',
    trigger_count     INTEGER NOT NULL DEFAULT 0,
    UNIQUE (service_name, name)
);
