-- Per-principal archived inbox items (alert history ids and synthetic ids of non-alert items).
CREATE TABLE inbox_archive (
    principal TEXT NOT NULL,
    item_id   TEXT NOT NULL,
    PRIMARY KEY (principal, item_id)
);
