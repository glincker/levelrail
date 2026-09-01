-- IAM-style policy documents, additive on top of the flat ability list
-- (api_tokens.abilities / users.abilities): a policy narrows a broad
-- ability down to specific resources (Deny) or grants an ability
-- scoped to specific resources without granting it globally (Allow).
CREATE TABLE iam_policies (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	document TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

-- One policy can attach to many principals and vice versa; principal_type
-- is "user" or "token", principal_id is the matching users.id/api_tokens.id.
-- No foreign key to either table: a principal_type is a discriminator, not
-- a single referenceable table, matching how audit_log.actor_type/actor_id
-- already handles the same polymorphic-reference shape in this codebase.
CREATE TABLE iam_policy_attachments (
	id TEXT PRIMARY KEY,
	policy_id TEXT NOT NULL REFERENCES iam_policies(id) ON DELETE CASCADE,
	principal_type TEXT NOT NULL,
	principal_id TEXT NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE (policy_id, principal_type, principal_id)
);

CREATE INDEX idx_iam_policy_attachments_principal ON iam_policy_attachments (principal_type, principal_id);
