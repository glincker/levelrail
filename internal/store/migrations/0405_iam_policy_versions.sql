-- Every saved document of an IAM policy, so a change can be diffed and
-- reviewed. The live document stays in iam_policies; this table is history.
CREATE TABLE iam_policy_versions (
	id TEXT PRIMARY KEY,
	policy_id TEXT NOT NULL REFERENCES iam_policies(id) ON DELETE CASCADE,
	version INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	document TEXT NOT NULL,
	actor TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	UNIQUE (policy_id, version)
);

INSERT INTO iam_policy_versions (id, policy_id, version, name, description, document, actor, created_at)
SELECT 'polv_' || id || '_1', id, 1, name, description, document, '', updated_at FROM iam_policies;
