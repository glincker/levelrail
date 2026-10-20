-- Branch deploys: a stable per-branch deployment of an app, separate from the
-- per pull request previews. One policy row per app, one row per live branch.
CREATE TABLE branch_deploy_settings (
    app_name   TEXT PRIMARY KEY,
    mode       TEXT NOT NULL DEFAULT 'off',
    patterns   TEXT NOT NULL DEFAULT '[]',
    updated_at TEXT NOT NULL
);

CREATE TABLE branch_deploys (
    id            TEXT PRIMARY KEY,
    app_name      TEXT NOT NULL,
    branch        TEXT NOT NULL,
    slug          TEXT NOT NULL,
    deploy_app_id TEXT NOT NULL,
    head_sha      TEXT NOT NULL DEFAULT '',
    head_message  TEXT NOT NULL DEFAULT '',
    domain        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL,
    status_reason TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE (app_name, slug)
);
CREATE INDEX branch_deploys_app_branch ON branch_deploys (app_name, branch);
