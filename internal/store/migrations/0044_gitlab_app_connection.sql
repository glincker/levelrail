-- Single-row GitLab OAuth Application connection, the GitLab counterpart
-- of github_app_connections (migrations/0034): instance_url/client_id
-- are not secret and live here as plain columns; client_secret and the
-- OAuth access_token/refresh_token/token_expires_at go through
-- internal/secrets.Manager instead, keyed by store.GitLabAppSecretsKey()
-- (internal/store/gitlab_app.go).
CREATE TABLE gitlab_app_connections (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    instance_url TEXT NOT NULL,
    client_id    TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
