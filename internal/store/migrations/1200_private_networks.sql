CREATE TABLE private_networks (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    project_id   TEXT NOT NULL DEFAULT '',
    default_deny INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL
);

CREATE TABLE private_network_members (
    network_id TEXT NOT NULL,
    app_id     TEXT NOT NULL,
    PRIMARY KEY (network_id, app_id)
);

CREATE TABLE private_network_rules (
    id         TEXT PRIMARY KEY,
    from_app   TEXT NOT NULL,
    to_app     TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE (from_app, to_app)
);
