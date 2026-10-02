-- A custom template is an operator-defined, shareable one-click deploy
-- captured from an already-running app's desired state
-- (internal/compose.FromDesiredServices), distinct from
-- internal/catalog's static built-in catalog. compose never holds a
-- real secret value: see that function's own doc comment.
CREATE TABLE custom_templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    compose     TEXT NOT NULL,
    source_app  TEXT NOT NULL DEFAULT '',
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
