-- Per-app form capture: form definitions and the submissions they collect.
-- ip_hash is a salted hash, never the raw address.
CREATE TABLE forms (
    id                 TEXT PRIMARY KEY,
    app_name           TEXT NOT NULL,
    name               TEXT NOT NULL,
    redirect_url       TEXT NOT NULL DEFAULT '',
    honeypot_field     TEXT NOT NULL DEFAULT 'bot-field',
    notify_channel_ids TEXT NOT NULL DEFAULT '[]',
    ip_salt            TEXT NOT NULL,
    created_at         TEXT NOT NULL,
    UNIQUE (app_name, name)
);

CREATE TABLE form_submissions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    form_id     TEXT NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    fields      TEXT NOT NULL,
    spam        INTEGER NOT NULL DEFAULT 0,
    spam_reason TEXT NOT NULL DEFAULT '',
    ip_hash     TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE INDEX form_submissions_form_created ON form_submissions (form_id, created_at DESC);
