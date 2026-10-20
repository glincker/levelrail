-- Arbitrary, operator-defined labels for organizing and filtering apps
-- independent of the project/environment hierarchy (organization ->
-- project -> environment already exists; tags cut across it), mirroring
-- Coolify's own Tag model. Apps-only for now: a database-tags join table
-- is a clean, additive follow-up if the need shows up, not added
-- speculatively here.
CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE app_tags (
    tag_id     TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    app_name   TEXT NOT NULL REFERENCES desired_services(name) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    PRIMARY KEY (tag_id, app_name)
);

CREATE INDEX idx_app_tags_app_name ON app_tags(app_name);
