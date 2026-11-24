-- One row per env edit of an app: the key names in the clear, the full variable map only as an encrypted blob.
CREATE TABLE env_revisions (
    app_name TEXT NOT NULL,
    rev INTEGER NOT NULL,
    at TEXT NOT NULL,
    actor TEXT NOT NULL DEFAULT '',
    names TEXT NOT NULL DEFAULT '[]',
    snapshot BLOB,
    PRIMARY KEY (app_name, rev)
);
