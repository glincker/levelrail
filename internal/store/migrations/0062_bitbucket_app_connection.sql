-- Single-row Bitbucket Cloud OAuth consumer connection, the Bitbucket
-- counterpart of gitlab_app_connections (migrations/0044). No
-- instance_url column: Bitbucket Cloud only, no self-hosted variant
-- (docs/design/git-provider-integrations.md section 3). No credential
-- columns: the consumer's own secret and the OAuth access_token/
-- refresh_token/token_expires_at all live in internal/secrets instead,
-- under BitbucketAppSecretsKey().
CREATE TABLE bitbucket_app_connections (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    oauth_key  TEXT NOT NULL,
    created_at TEXT NOT NULL
);
