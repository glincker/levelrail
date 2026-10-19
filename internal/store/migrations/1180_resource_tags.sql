CREATE TABLE resource_tags (
    tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (tag_id, kind, name)
);

CREATE INDEX idx_resource_tags_resource ON resource_tags(kind, name);

CREATE TABLE resource_views (
    id         TEXT PRIMARY KEY,
    owner      TEXT NOT NULL,
    name       TEXT NOT NULL,
    query      TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (owner, name)
);
