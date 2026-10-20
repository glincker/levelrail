-- Pending two-person approval for a deploy/promote into a protected
-- environment (internal/api/deploy_approvals.go): replaces the old
-- same-actor confirm-flag gate (environments.go's environmentNeedsConfirmation)
-- with a real record a second, differently-privileged user must act on
-- before the reconcile path runs. service_name is always the app whose
-- desired image actually changes once approved; source_service_name is
-- only set for action = 'promote', naming the app the image came from.
CREATE TABLE deploy_approvals (
    id                    TEXT PRIMARY KEY,
    service_name          TEXT NOT NULL,
    source_service_name   TEXT NOT NULL DEFAULT '',
    environment_id        TEXT NOT NULL,
    action                TEXT NOT NULL,
    image                 TEXT NOT NULL,
    status                TEXT NOT NULL DEFAULT 'pending',
    requested_by_type     TEXT NOT NULL,
    requested_by          TEXT NOT NULL,
    requested_by_name     TEXT NOT NULL,
    approved_by_type      TEXT NOT NULL DEFAULT '',
    approved_by           TEXT NOT NULL DEFAULT '',
    approved_by_name      TEXT NOT NULL DEFAULT '',
    reason                TEXT NOT NULL DEFAULT '',
    created_at            TEXT NOT NULL,
    expires_at            TEXT NOT NULL,
    decided_at            TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_deploy_approvals_service ON deploy_approvals(service_name);
CREATE INDEX idx_deploy_approvals_status ON deploy_approvals(status);
