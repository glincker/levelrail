-- Deploy contexts: env and build overrides per app and context name
-- (deploy-preview, branch-deploy, or branch:<pattern>). Additive; existing
-- preview and branch env override tables are untouched and still apply.
CREATE TABLE app_context_config (
    app_name      TEXT NOT NULL,
    context       TEXT NOT NULL,
    env           TEXT NOT NULL DEFAULT '{}',
    build_command TEXT NOT NULL DEFAULT '',
    publish_dir   TEXT NOT NULL DEFAULT '',
    build_args    TEXT NOT NULL DEFAULT '{}',
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (app_name, context)
);
