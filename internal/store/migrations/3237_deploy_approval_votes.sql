-- Required approvers on protected environments. One row per distinct
-- approver principal per approval; the primary key makes a retry or a double
-- click a no-op instead of a second vote.
CREATE TABLE deploy_approval_votes (
    approval_id   TEXT NOT NULL,
    approver_key  TEXT NOT NULL,
    approver_type TEXT NOT NULL,
    approver_id   TEXT NOT NULL,
    approver_name TEXT NOT NULL DEFAULT '',
    binding       TEXT NOT NULL,
    image_digest  TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    PRIMARY KEY (approval_id, approver_key)
);

-- Per environment approver count. A missing row means one approver, so every
-- existing environment keeps the single approval behavior.
CREATE TABLE environment_approval_policies (
    environment_id     TEXT PRIMARY KEY,
    required_approvers INTEGER NOT NULL DEFAULT 1,
    updated_at         TEXT NOT NULL DEFAULT ''
);
