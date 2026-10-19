-- Unified search index: one row per searchable thing, fed by providers.
CREATE TABLE IF NOT EXISTS search_docs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    ref        TEXT NOT NULL UNIQUE,
    kind       TEXT NOT NULL,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '',
    tags       TEXT NOT NULL DEFAULT '',
    app        TEXT NOT NULL DEFAULT '',
    env        TEXT NOT NULL DEFAULT '',
    node       TEXT NOT NULL DEFAULT '',
    prefix     TEXT NOT NULL,
    name       TEXT NOT NULL,
    ability    TEXT NOT NULL,
    confirm    INTEGER NOT NULL DEFAULT 0,
    href       TEXT NOT NULL DEFAULT '',
    action_id  TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0,
    hash       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS search_docs_kind ON search_docs (kind);

CREATE VIRTUAL TABLE IF NOT EXISTS search_docs_fts USING fts5(
    title, body,
    content='search_docs', content_rowid='id', tokenize='trigram'
);

CREATE TRIGGER IF NOT EXISTS search_docs_ai AFTER INSERT ON search_docs BEGIN
    INSERT INTO search_docs_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
END;
CREATE TRIGGER IF NOT EXISTS search_docs_ad AFTER DELETE ON search_docs BEGIN
    INSERT INTO search_docs_fts (search_docs_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
END;
CREATE TRIGGER IF NOT EXISTS search_docs_au AFTER UPDATE ON search_docs BEGIN
    INSERT INTO search_docs_fts (search_docs_fts, rowid, title, body) VALUES ('delete', old.id, old.title, old.body);
    INSERT INTO search_docs_fts (rowid, title, body) VALUES (new.id, new.title, new.body);
END;

CREATE TABLE IF NOT EXISTS search_usage (
    principal_id TEXT NOT NULL,
    ref          TEXT NOT NULL,
    uses         INTEGER NOT NULL DEFAULT 0,
    last_used_at INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (principal_id, ref)
);

CREATE TABLE IF NOT EXISTS search_saved (
    id           TEXT PRIMARY KEY,
    principal_id TEXT NOT NULL,
    name         TEXT NOT NULL,
    query        TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    UNIQUE (principal_id, name)
);
