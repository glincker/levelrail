-- Email templates, audiences, suppressions and broadcasts.
CREATE TABLE mail_templates (
    event_key  TEXT PRIMARY KEY,
    subject    TEXT NOT NULL,
    preheader  TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL,
    updated_by TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);

CREATE TABLE mail_template_versions (
    event_key  TEXT NOT NULL,
    version    INTEGER NOT NULL,
    subject    TEXT NOT NULL,
    preheader  TEXT NOT NULL DEFAULT '',
    body       TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (event_key, version)
);

CREATE TABLE mail_layout (
    id               INTEGER PRIMARY KEY CHECK (id = 1),
    header_image_url TEXT NOT NULL DEFAULT '',
    footer_text      TEXT NOT NULL DEFAULT ''
);
INSERT INTO mail_layout (id) VALUES (1);

CREATE TABLE mail_groups (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    rules       TEXT NOT NULL DEFAULT '{}',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE mail_suppressions (
    email      TEXT PRIMARY KEY,
    reason     TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE mail_broadcasts (
    id           TEXT PRIMARY KEY,
    subject      TEXT NOT NULL,
    body         TEXT NOT NULL,
    template_key TEXT NOT NULL DEFAULT '',
    group_id     TEXT NOT NULL DEFAULT '',
    user_ids     TEXT NOT NULL DEFAULT '[]',
    status       TEXT NOT NULL,
    scheduled_at TEXT NOT NULL DEFAULT '',
    total        INTEGER NOT NULL DEFAULT 0,
    sent         INTEGER NOT NULL DEFAULT 0,
    failed       INTEGER NOT NULL DEFAULT 0,
    suppressed   INTEGER NOT NULL DEFAULT 0,
    error        TEXT NOT NULL DEFAULT '',
    created_by   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    started_at   TEXT NOT NULL DEFAULT '',
    finished_at  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_mail_broadcasts_status ON mail_broadcasts (status);

CREATE TABLE mail_broadcast_recipients (
    broadcast_id TEXT NOT NULL REFERENCES mail_broadcasts(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    user_id      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL,
    error        TEXT NOT NULL DEFAULT '',
    sent_at      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (broadcast_id, email)
);
