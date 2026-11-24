-- Mined log templates per deploy: one row per cluster with a masked sample, so repeated failures read as a short list.
CREATE TABLE deploy_log_templates (
    deploy_id   TEXT NOT NULL,
    template_id TEXT NOT NULL,
    template    TEXT NOT NULL,
    count       INTEGER NOT NULL,
    sample      TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    PRIMARY KEY (deploy_id, template_id)
);
