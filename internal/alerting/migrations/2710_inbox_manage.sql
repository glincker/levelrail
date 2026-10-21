-- Per-principal deleted inbox items (reversible) and the time an item was archived, for retention.
CREATE TABLE inbox_deleted (
    principal  TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    deleted_at TEXT NOT NULL,
    PRIMARY KEY (principal, item_id)
);

ALTER TABLE inbox_archive ADD COLUMN archived_at TEXT NOT NULL DEFAULT '';
